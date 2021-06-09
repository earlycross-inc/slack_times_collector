package times

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

func HandleAppHomeOpen(w http.ResponseWriter, r *http.Request) {
	if err := setupSlackAPI(); err != nil {
		log.Printf("setup error: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	botUserID := os.Getenv("SLACK_BOT_USERID")
	timesNewsChanID := os.Getenv("SLACK_TIMES_NEWS_CHANNEL_ID")
	if botUserID == "" || timesNewsChanID == "" {
		log.Printf("setup error: env vars are not set")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ev, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch ev.Type {
	case slackevents.URLVerification:
		onURLVerification(w, body)
		return

	case slackevents.CallbackEvent:
		innerEv := ev.InnerEvent
		var evErr error

		switch ev := innerEv.Data.(type) {
		case *slackevents.AppHomeOpenedEvent:
			evErr = onAppHomeOpened(ev, botUserID, timesNewsChanID)
		}

		if evErr != nil {
			log.Printf("failed to process event from slack: %v", evErr)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
}

// SlackにURLを登録する際に行われるVerificationに対応する
func onURLVerification(w http.ResponseWriter, body []byte) {
	var r *slackevents.ChallengeResponse
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(r.Challenge))
}

type channelWatchState struct {
	channel    slack.Channel
	isWatching bool
}

func onAppHomeOpened(ev *slackevents.AppHomeOpenedEvent, botUserID string, timesNewsChanID string) error {
	log.Println("app home opened")
	slackChans, err := getAllConversations()
	if err != nil {
		return fmt.Errorf("failed to get channels: %w", err)
	}
	timesChans := slackChans.selectTimesChannelsOfUser(ev.User, timesNewsChanID)

	watchStates := make([]*channelWatchState, 0)
	for _, c := range timesChans {
		isWatching, err := checkIfWatchingChannel(&c, botUserID)
		if err != nil {
			return fmt.Errorf("failed to fetch channel watching state: %w", err)
		}
		watchStates = append(watchStates, &channelWatchState{channel: c, isWatching: isWatching})
	}

	if err := publishAppHomeView(ev.User, watchStates); err != nil {
		return fmt.Errorf("failed to publish app home view: %w", err)
	}
	return nil
}

func checkIfWatchingChannel(channel *slack.Channel, botUserID string) (bool, error) {
	params := &slack.GetUsersInConversationParameters{
		ChannelID: channel.ID,
	}
	users, _, err := slackAPI.GetUsersInConversation(params)
	if err != nil {
		return false, err
	}
	for _, userID := range users {
		if userID == botUserID {
			return true, nil
		}
	}
	return false, nil
}

func publishAppHomeView(userID string, channelWatchStates []*channelWatchState) error {
	view := buildAppHomeView(channelWatchStates)

	resp, err := slackAPI.PublishView(userID, view, "")
	log.Printf("Publish view response: %+v\n", *resp)
	return err
}

func buildAppHomeView(chanWatchStates []*channelWatchState) slack.HomeTabViewRequest {
	headerTxt := slack.NewTextBlockObject("plain_text", "Times News :newspaper:", true, false)
	header := slack.NewHeaderBlock(headerTxt)

	descTxt := slack.NewTextBlockObject("mrkdwn", "定期的にみんなのTimesの更新をチェックして、#times-news に更新情報を流す君です", false, false)
	descSec := slack.NewSectionBlock(descTxt, nil, nil)

	divider := slack.NewDividerBlock()

	var chansTxt *slack.TextBlockObject
	if len(chanWatchStates) == 0 {
		chansTxt = slack.NewTextBlockObject("mrkdwn", "あなたのTimesチャンネルが見つかりませんでした:thinking_face: 名前が\"times-\"から始まるチャンネルを作ってみてください。", false, false)
	} else {
		chansTxt = slack.NewTextBlockObject("mrkdwn", "あなたのTimesチャンネルです。右のボタンを押すと監視状態を変更できます。", false, false)
	}
	chansTxtSec := slack.NewSectionBlock(chansTxt, nil, nil)

	watchStateToggles := buildWatchStateToggleButtonsBlock(chanWatchStates)

	blkSet := []slack.Block{header, descSec, divider, chansTxtSec}
	blkSet = append(blkSet, watchStateToggles...)

	return slack.HomeTabViewRequest{
		Type:   slack.VTHomeTab,
		Blocks: slack.Blocks{BlockSet: blkSet},
	}
}

func buildWatchStateToggleButtonsBlock(chanWatchStates []*channelWatchState) []slack.Block {
	blks := make([]slack.Block, 0, len(chanWatchStates))
	for _, ch := range chanWatchStates {
		chanProps := &channelProps{ID: ch.channel.ID, Name: ch.channel.Name}
		if ch.isWatching {
			blks = append(blks, buildStopWatchingButton(chanProps))
		} else {
			blks = append(blks, buildApproveWatchingButton(chanProps))
		}
	}
	return blks
}
