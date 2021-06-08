package times

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/slack-go/slack"
)

var (
	slackAPI *slack.Client
)

func setupSlackAPI() error {
	token := os.Getenv("SLACK_BOT_TOKEN")
	if token == "" {
		return errors.New("token is not set")
	}
	slackAPI = slack.New(token)
	return nil
}

const (
	actionStopWatching    = "stop_watching"
	actionApproveWatching = "approve_watching"
)

type slackChannels []slack.Channel

func (cs slackChannels) selectTimesChannelsOfUser(userID string, timesNewsChanID string) slackChannels {
	res := make(slackChannels, 0)
	for _, ch := range cs {
		if ch.Creator == userID && strings.HasPrefix(ch.Name, "times-") && ch.ID != timesNewsChanID {
			res = append(res, ch)
		}
	}
	return res
}

func (cs slackChannels) selectTimesChannels(timesNewsChanID string) slackChannels {
	res := make(slackChannels, 0)
	for _, ch := range cs {
		if strings.HasPrefix(ch.Name, "times-") && ch.ID != timesNewsChanID {
			res = append(res, ch)
		}
	}
	return res
}

type channelProps struct {
	ID   string
	Name string
}

func newChannelProps(chanID string, chanName string) *channelProps {
	return &channelProps{
		ID:   chanID,
		Name: chanName,
	}
}

func (cp *channelProps) String() string {
	return cp.ID + "|" + cp.Name
}

func parseChannelProps(s string) (*channelProps, error) {
	split := strings.SplitN(s, "|", 2)
	if len(split) != 2 {
		return nil, fmt.Errorf("can't be parsed as channelProps: %s", s)
	}
	return &channelProps{
		ID:   split[0],
		Name: split[1],
	}, nil
}

// button for channel which is currently watched
func buildStopWatchingButton(cp *channelProps) *slack.SectionBlock {
	labeltxt := slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("#%s: %s", cp.Name, "*監視中* :eyes:"), false, false)
	btnTxt := slack.NewTextBlockObject("plain_text", "監視を中止", false, false)
	button := slack.NewButtonBlockElement(actionStopWatching, cp.String(), btnTxt)
	button.Style = slack.StyleDanger
	return slack.NewSectionBlock(labeltxt, nil, slack.NewAccessory(button))
}

// button for channel which is not currently watched
func buildApproveWatchingButton(cp *channelProps) *slack.SectionBlock {
	labeltxt := slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("#%s: %s", cp.Name, "*未監視* :see_no_evil:"), false, false)
	btnTxt := slack.NewTextBlockObject("plain_text", "監視を許可", false, false)
	button := slack.NewButtonBlockElement(actionApproveWatching, cp.String(), btnTxt)
	button.Style = slack.StylePrimary
	return slack.NewSectionBlock(labeltxt, nil, slack.NewAccessory(button))
}

// get all public channels using cursor
func getAllConversations() (slackChannels, error) {
	res := make(slackChannels, 0)
	cursor := ""
	for {
		chans, nextCursor, err := slackAPI.GetConversations(&slack.GetConversationsParameters{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		res = append(res, chans...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}
	return res, nil
}
