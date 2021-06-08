package times

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"

	"github.com/slack-go/slack"
)

func HandleToggleWatchStateRequest(w http.ResponseWriter, r *http.Request) {
	if err := setupSlackAPI(); err != nil {
		log.Printf("setup error: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var ev slack.InteractionCallback
	if err := json.Unmarshal([]byte(r.FormValue("payload")), &ev); err != nil {
		log.Printf("failed to unmarshal payload as InteractionCallback: %v\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// 押されたボタンに応じて監視状態を変更する
	if len(ev.ActionCallback.BlockActions) > 0 {
		action := ev.ActionCallback.BlockActions[0]
		log.Printf("block_id: %s, action_id: %s, value: %s\n", action.BlockID, action.ActionID, action.Value)

		// ボタンに紐つけられた対象Channelのデータを取得
		chProps, err := parseChannelProps(action.Value)
		if err != nil {
			log.Printf("%v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 監視状態を変更し、押されたボタンのBlockを置き換える新しいBlockを生成
		var newBtn slack.Block

		switch action.ActionID {
		case actionApproveWatching:
			newBtn, err = onApproveWatching(chProps)
		case actionStopWatching:
			newBtn, err = onStopWatching(chProps)
		default:
			newBtn = nil
			err = fmt.Errorf("invalid action id: %s\n", action.ActionID)
		}

		// エラーが起きた場合はエラーモーダルを表示して終了
		if err != nil {
			log.Printf("failed to update watch state: %v\n", err)
			modalView := buildErrorModal(chProps)
			if err := publishResponseModal(ev.TriggerID, modalView); err != nil {
				log.Printf("failed to open modal: %v\n", err)
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		// 状態変更完了モーダルを表示
		modalView := buildResponseModal(action.ActionID, chProps)
		if err := publishResponseModal(ev.TriggerID, modalView); err != nil {
			log.Printf("failed to open modal: %v\n", err)
		}

		// ボタンを置き換えた新しいAppHomeViewを表示
		newView := buildUpdatedAppHomeView(ev.View.Blocks.BlockSet, action.BlockID, newBtn)
		if err := publishUpdatedAppHomeView(ev.User.ID, newView, ev.View.Hash); err != nil {
			log.Printf("failed to update app home view: %v\n", err)
		}
	}
	w.WriteHeader(http.StatusOK)
}

// 監視許可: 指定されたChannelに参加
func onApproveWatching(chProps *channelProps) (slack.Block, error) {
	_, _, warnings, err := slackAPI.JoinConversation(chProps.ID)
	if err != nil {
		return nil, err
	}
	log.Println("warnings for joinning conversation")
	for _, w := range warnings {
		log.Println(w)
	}

	// 監視許可を置き換えるのは監視停止ボタン
	return buildStopWatchingButton(chProps), nil
}

// 監視停止: 指定されたChannelから退出
func onStopWatching(chProps *channelProps) (slack.Block, error) {
	if _, err := slackAPI.LeaveConversation(chProps.ID); err != nil {
		return nil, err
	}

	// 監視停止を置き換えるのは監視許可ボタン
	return buildApproveWatchingButton(chProps), nil
}

func publishResponseModal(triggerID string, modalView slack.ModalViewRequest) error {
	resp, err := slackAPI.OpenView(triggerID, modalView)
	log.Printf("OpenView resp: %+v\n", resp)
	return err
}

func buildResponseModal(actionID string, chProps *channelProps) slack.ModalViewRequest {
	titleTxt := slack.NewTextBlockObject("plain_text", "Succeeded!", false, false)
	closeTxt := slack.NewTextBlockObject("plain_text", "Close", false, false)

	var (
		bodyTxt *slack.TextBlockObject
	)
	switch actionID {
	case actionApproveWatching:
		bodyTxt = slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("#%s の監視を開始しました :eyes:", chProps.Name), false, false)
	case actionStopWatching:
		bodyTxt = slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("#%s の監視を中止しました :see_no_evil:", chProps.Name), false, false)
	}

	bodyTxtSec := slack.NewSectionBlock(bodyTxt, nil, nil)
	body := slack.Blocks{
		BlockSet: []slack.Block{bodyTxtSec},
	}

	return slack.ModalViewRequest{
		Type:   slack.VTModal,
		Title:  titleTxt,
		Close:  closeTxt,
		Blocks: body,
	}
}

func buildErrorModal(chProps *channelProps) slack.ModalViewRequest {
	titleTxt := slack.NewTextBlockObject("plain_text", "Error", false, false)
	closeTxt := slack.NewTextBlockObject("plain_text", "Close", false, false)

	bodyTxt := slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("#%s の監視状態の変更に失敗しました :dizzy_face:", chProps.Name), false, false)
	bodyTxtSec := slack.NewSectionBlock(bodyTxt, nil, nil)
	body := slack.Blocks{
		BlockSet: []slack.Block{bodyTxtSec},
	}

	return slack.ModalViewRequest{
		Type:   slack.VTModal,
		Title:  titleTxt,
		Close:  closeTxt,
		Blocks: body,
	}
}

func publishUpdatedAppHomeView(userID string, newView slack.HomeTabViewRequest, hash string) error {
	resp, err := slackAPI.PublishView(userID, newView, hash)
	log.Printf("Publish view response: %+v\n", *resp)
	return err
}

func buildUpdatedAppHomeView(prevBlocks []slack.Block, updBlockID string, newBtn slack.Block) slack.HomeTabViewRequest {
	newBlocks := make([]slack.Block, 0, len(prevBlocks))

	// 押されたボタンに対応するIDを持つBlockを置き換える
	for _, blk := range prevBlocks {
		rv := reflect.ValueOf(blk)
		blkID := rv.Elem().FieldByName("BlockID")
		if !blkID.IsZero() && blkID.String() == updBlockID {
			newBlocks = append(newBlocks, newBtn)
		} else {
			newBlocks = append(newBlocks, blk)
		}
	}

	return slack.HomeTabViewRequest{
		Type:   slack.VTHomeTab,
		Blocks: slack.Blocks{BlockSet: newBlocks},
	}
}
