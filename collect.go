package times

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

type PubSubMessage struct {
	Data []byte `json:"data"`
}

type timesStat struct {
	chanID  string
	postCnt int
}

func CollectSlackTimesNews(ctx context.Context, m PubSubMessage) error {
	if err := setupSlackAPI(); err != nil {
		log.Printf("%v\n", err)
		return nil
	}
	timesNewsChanID := os.Getenv("SLACK_TIMES_NEWS_CHANNEL_ID")
	if timesNewsChanID == "" {
		log.Printf("times channel ID is not set")
		return nil
	}

	log.Println("start collecting times")
	log.Println("current time:", time.Now().String())

	hourAgo := time.Now().Add(time.Hour * -1)

	chans, err := getAllConversations()
	if err != nil {
		log.Printf("failed to get channels list: %v\n", err)
		return nil
	}

	timesStats := make([]timesStat, 0)
	for _, ch := range chans.selectTimesChannels(timesNewsChanID) {
		postCnt, err := getChannelPostCountAfter(ch.ID, hourAgo)
		if err != nil {
			if err == NotInChannelError {
				// not in channelエラー -> 未監視のChannelなのでスキップ
				log.Printf("unwatched channel: #%s(ID: %s)\n", ch.Name, ch.ID)
				continue
			}
			// その他のエラーの場合は処理を中止
			log.Printf("failed to get conversation history: %v (channel ID: %s)\n", err, ch.ID)
			return nil
		}
		if postCnt != 0 {
			timesStats = append(timesStats, timesStat{chanID: ch.ID, postCnt: postCnt})
		} else {
			log.Printf("no post within last hour in channel: #%s(ID: %s)\n", ch.Name, ch.ID)
		}
	}
	if len(timesStats) == 0 {
		log.Println("no times posts within last hour")
		return nil
	}

	log.Printf("times stats: %+v", timesStats)
	sort.Slice(timesStats, func(i, j int) bool {
		return timesStats[i].postCnt < timesStats[j].postCnt
	})

	msg := buildTimesNewsMessage(timesStats)
	if _, _, err := slackAPI.PostMessage(timesNewsChanID, slack.MsgOptionText(msg, false)); err != nil {
		log.Printf("failed to post message: %v\n", err)
		return nil
	}
	return nil
}

var (
	NotInChannelError = errors.New("not in channel")
)

// 指定Channelの指定時刻以降の投稿数を取得
func getChannelPostCountAfter(chanID string, t time.Time) (int, error) {
	cursor := ""
	oldest := fmt.Sprintf("%d", t.Unix())
	cnt := 0

	for {
		resp, err := slackAPI.GetConversationHistory(&slack.GetConversationHistoryParameters{
			ChannelID: chanID,
			Cursor:    cursor,
			Inclusive: true,
			Oldest:    oldest,
		})
		if err != nil {
			if resp != nil && resp.Error == "not_in_channel" {
				return 0, NotInChannelError
			}
			return 0, err
		}

		for _, msg := range resp.Messages {
			// (システムメッセージではない)通常の投稿はSubTypeが空
			if msg.Type == "message" && msg.SubType == "" {
				cnt += 1
			}
		}

		if resp.ResponseMetadata.Cursor == "" {
			return cnt, nil
		}
		cursor = resp.ResponseMetadata.Cursor
	}
}

func buildTimesNewsMessage(timesStats []timesStat) string {
	msgText := new(strings.Builder)
	fmt.Fprintln(msgText, "過去1時間のTimes投稿数:")
	for _, ts := range timesStats {
		fmt.Fprintf(msgText, "• <#%s>: *%d件*\n", ts.chanID, ts.postCnt)
	}
	return msgText.String()
}
