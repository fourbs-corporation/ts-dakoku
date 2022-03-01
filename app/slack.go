package app

import (
	"time"
	"strings"
	"strconv"
	"math"

	"log"

	"github.com/nlopes/slack"

)

const (
	actionTypeAttend           = "attend"
	actionTypeRest             = "rest"
	actionTypeUnrest           = "unrest"
	actionTypeLeave            = "leave"
	actionTypeReset            = "reset"
	actionTypeOnTime           = "ontime"
	actionTypeBulkMonth        = "bulk-month"
	actionTypeSelectChannel    = "select-channel"
	actionTypeUnselectChannel  = "unselect-channel"
	callbackIDChannelSelect    = "slack_channel_select_button"
	callbackIDAttendanceButton = "attendance_button"
)

func (ctx *Context) getActionCallback(data *slack.AttachmentActionCallback) (*slack.Msg, string, error) {
	ctx.UserID = data.User.ID
	client := ctx.createTimeTableClient()
	timeTable, err := client.GetTimeTable()
	log.Printf("-- timeTable.IsRest --")
	log.Print(timeTable.Items)
	if err != nil {
		state := State{
			TeamID:      data.Team.ID,
			UserID:      ctx.UserID,
			ResponseURL: data.ResponseURL,
		}
		err, msg := ctx.getLoginSlackMessage(state)
		return err, data.ResponseURL, msg
	}

	text := ""
	now := time.Now()
	year, month, day := now.Date()
	selectedTime := time.Now() // 初期化
	selectedTimeStr := selectedTime.Format("2006/01/02") // 初期化
	if data.Actions[0].Type == "select" {
		selectedValue := data.Actions[0].SelectedOptions[0].Value // 選択した出勤時間を取得
		timeFactor := strings.Split(selectedValue, ":") // 時刻文字列を「:」で分割
		hour, _ := strconv.Atoi(timeFactor[0]) // string to int
		min, _ := strconv.Atoi(timeFactor[1]) // string to int
		selectedTime = time.Date(year, month, day, hour, min, 0, 0, time.UTC)
		selectedTimeStr = selectedTime.Format("2006/01/02 15:04") // 日付を文字列化
	}
	attendance := -1
	switch data.Actions[0].Name {
	case actionTypeReset:
		{
			timeTable.Reset(selectedTime)
			text = "【" + selectedTimeStr + "】" + "の勤怠情報をリセットしました :u7a7a:"
		}
	case actionTypeOnTime:
		{
			startTime := time.Date(year, month, day, 9, 0, 0, 0, time.UTC) // 定時出勤時刻
			endTime := time.Date(year, month, day, 18, 0, 0, 0, time.UTC) // 定時退勤時刻
			restStartTime := time.Date(year, month, day, 12, 0, 0, 0, time.UTC) // 定時休憩開始時刻
			restEndTime := time.Date(year, month, day, 13, 0, 0, 0, time.UTC) // 定時休憩終了時刻
			timeTable.Attend(startTime)
			timeTable.Rest(restStartTime)
			timeTable.Unrest(restEndTime)
			timeTable.Leave(endTime)
			text = "【" + selectedTimeStr + "】" + "定時で勤怠入力しました :high_brightness:"
		}
	case actionTypeBulkMonth:
		{
			t := time.Date(year, month + 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1) // 今月の日数
			// startTime := time.Date(year, month, day, 9, 0, 0, 0, time.UTC) // 定時出勤時刻
			// endTime := time.Date(year, month, day, 18, 0, 0, 0, time.UTC) // 定時退勤時刻
			// restStartTime := time.Date(year, month, day, 12, 0, 0, 0, time.UTC) // 定時休憩開始時刻
			// restEndTime := time.Date(year, month, day, 13, 0, 0, 0, time.UTC) // 定時休憩終了時刻
			// timeTable.Attend(startTime)
			// timeTable.Rest(restStartTime)
			// timeTable.Unrest(restEndTime)
			// timeTable.Leave(endTime)
			// text = "【" + selectedTimeStr + "】" + "定時で勤怠入力しました :high_brightness:"
			text = strconv.Itoa(t.Day())
		}
	case actionTypeLeave:
		{
			if !timeTable.HasRested() {
				restStartTime := time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
				restEndTime := time.Date(year, month, day, 13, 0, 0, 0, time.UTC)
				timeTable.Rest(restStartTime)
				timeTable.Unrest(restEndTime)
			}
			// attendance = 0
			timeTable.Leave(selectedTime)
			text = "【" + selectedTimeStr + "】" + "退勤しました :house:"
		}
	case actionTypeRest:
		{
			timeTable.Rest(selectedTime)
			text = "【" + selectedTimeStr + "】" + "休憩を開始しました :coffee:"
		}
	case actionTypeUnrest:
		{
			timeTable.Unrest(selectedTime)
			text =  "【" + selectedTimeStr + "】" + "休憩を終了しました :computer:"
		}
	case actionTypeAttend:
		{
			// attendance = 1
			timeTable.Attend(selectedTime)
			text = "【" + selectedTimeStr + "】" + "出勤しました :office:"
		}
	}

	params := &slack.Msg{
		ResponseType:    "in_channel",
		ReplaceOriginal: true,
		Text:            text,
	}

	var ok bool
	if attendance != -1 {
		selectedTime := data.Actions[0].SelectedOptions[0].Value // 選択した出勤時間を取得
		timeFactor := strings.Split(selectedTime, ":") // 時刻文字列を「:」で分割
		hour, _ := strconv.Atoi(timeFactor[0]) // string to int
		min, _ := strconv.Atoi(timeFactor[1]) // string to int
		// attendTime := time.Date(year, month, day, hour, min, 0, 0, time.UTC)
		ok, err = client.SetAttendance(1, hour, min)
		// ok, err = client.SetAttendance(attendance == 1)
	} else {
		ok, err = client.UpdateTimeTable(timeTable)
	}
	if !ok || err != nil {
		params.ResponseType = "ephemeral"
		params.ReplaceOriginal = false
		params.Text = "勤務表の更新に失敗しました :warning:"
	}

	return params, data.ResponseURL, nil
}

func (ctx *Context) getLoginSlackMessage(state State) (*slack.Msg, error) {
	stateKey, err := ctx.storeState(state)
	if err != nil {
		return nil, err
	}
	return &slack.Msg{
		Attachments: []slack.Attachment{
			slack.Attachment{
				Text:       "TeamSpirit で認証を行って、再度 `/ts` コマンドを実行してください :bow:",
				CallbackID: callbackIDAttendanceButton,
				Actions: []slack.AttachmentAction{
					slack.AttachmentAction{
						Name:  "authenticate",
						Value: "authenticate",
						Text:  "認証する",
						Style: "primary",
						Type:  "button",
						URL:   ctx.getSalesforceAuthenticateURL(stateKey),
					},
				},
			},
		},
	}, nil
}

func (ctx *Context) getAuthenticateSlackMessage(state State) (*slack.Msg, error) {
	stateKey, err := ctx.storeState(state)
	if err != nil {
		return nil, err
	}
	return &slack.Msg{
		Attachments: []slack.Attachment{
			slack.Attachment{
				Text:       "Slack で認証を行って、再度 `/ts channel` コマンドを実行してください :bow:",
				CallbackID: "slack_authentication_button",
				Actions: []slack.AttachmentAction{
					slack.AttachmentAction{
						Name:  "slack-authenticate",
						Value: "slack-authenticate",
						Text:  "認証する",
						Style: "primary",
						Type:  "button",
						URL:   ctx.getSlackAuthenticateURL(state.TeamID, stateKey),
					},
				},
			},
		},
	}, nil
}

func (ctx *Context) getChannelSelectSlackMessage() (*slack.Msg, error) {
	return &slack.Msg{
		Attachments: []slack.Attachment{
			slack.Attachment{
				Text:       "打刻時に通知するチャネルを選択して下さい",
				CallbackID: callbackIDChannelSelect,
				Actions: []slack.AttachmentAction{
					slack.AttachmentAction{
						Name:       actionTypeSelectChannel,
						Value:      actionTypeSelectChannel,
						Text:       "チャネルを選択",
						Type:       "select",
						DataSource: "channels",
					},
					slack.AttachmentAction{
						Name:  actionTypeUnrest,
						Value: actionTypeUnrest,
						Text:  "通知を止める",
						Style: "danger",
						Type:  "button",
					},
				},
			},
		},
	}, nil
}

func convTimeHourColMin(timeByMin int) string {
	timeByHour := float64(timeByMin) / 60 // 単位：時
	hour := int( math.Floor(timeByHour) ) // 単位：時
	min := int( ( float64(timeByHour) - float64(hour) ) * 60 ) // 単位：分
	minStr := "00"
	if min > 0 {
		minStr = strconv.Itoa(min)
	}
	timeHourColMin := strconv.Itoa(hour) + ":" + minStr // string
	return timeHourColMin
}

func (ctx *Context) getSlackMessage(command slack.SlashCommand) (*slack.Msg, error) {
	text := command.Text
	state := State{
		TeamID:      command.TeamID,
		UserID:      command.UserID,
		ResponseURL: command.ResponseURL,
	}
	client := ctx.createTimeTableClient()
	if client.HTTPClient == nil || text == "login" {
		return ctx.getLoginSlackMessage(state)
	}
	timeTable, err := client.GetTimeTable()
	if err != nil {
		return ctx.getLoginSlackMessage(state)
	}
	if text == "channel" {
		if ctx.getSlackAccessTokenForUser() == "" {
			return ctx.getAuthenticateSlackMessage(state)
		}
		return ctx.getChannelSelectSlackMessage()
	}
	if text == "today" {
		now := time.Now()
		todayStr := now.Format("2006/01/02")
		slackMsg := "【" + todayStr + "】" + "\n"
		dakokuTime := 0
		dakokuTimeStr := ""
		items := timeTable.Items
		for _, item := range items {
			if item.IsAttendance() && item.From.Valid {
				dakokuTime = int(item.From.Int64)
				dakokuTimeStr = convTimeHourColMin(dakokuTime)
				slackMsg += "出勤時間: " + dakokuTimeStr + "\n"
			} else if item.IsAttendance() && !item.From.Valid {
				slackMsg += "出勤時間: 未入力" + "\n"
			}
			if item.IsAttendance() && item.To.Valid {
				dakokuTime = int(item.To.Int64)
				dakokuTimeStr = convTimeHourColMin(dakokuTime)
				slackMsg += "退勤時間: " + dakokuTimeStr + "\n"
			} else if item.IsAttendance() && !item.To.Valid {
				slackMsg += "退勤時間: 未入力" + "\n"
			}
			if item.IsRest() && item.From.Valid {
				dakokuTime = int(item.From.Int64)
				dakokuTimeStr = convTimeHourColMin(dakokuTime)
				slackMsg += "休憩開始: " + dakokuTimeStr + "\n"
			} else if item.IsRest() && !item.From.Valid {
				slackMsg += "休憩開始: 未入力" + "\n"
			}
			if item.IsRest() && item.To.Valid {
				dakokuTime = int(item.To.Int64)
				dakokuTimeStr = convTimeHourColMin(dakokuTime)
				slackMsg += "休憩終了: " + dakokuTimeStr + "\n"
			} else if item.IsRest() && !item.To.Valid {
				slackMsg += "休憩終了: 未入力" + "\n"
			}
		}
		return &slack.Msg{
			Text: slackMsg,
		}, nil
	}
	if text == "ontime" {
		return &slack.Msg{
			Text: "定時勤務（9:00 ~ 18:00, 休憩12:00 ~ 13:00）として勤怠を打刻します。",
			Attachments: []slack.Attachment{
				slack.Attachment{
					CallbackID: callbackIDAttendanceButton,
					Actions: []slack.AttachmentAction{
						slack.AttachmentAction{
							Name:  actionTypeOnTime,
							Value: actionTypeOnTime,
							Text:  "定時打刻する",
							Style: "primary",
							Type:  "button",
							Confirm: &slack.ConfirmationField{
								Text:        "本当に定時打刻しますか？",
								OkText:      "はい",
								DismissText: "いいえ",
							},
						},
					},
				},
			},
		}, nil	
	}
	if text == "bulk-month" {
		return &slack.Msg{
			Text: "今月の勤怠を定時勤務（9:00 ~ 18:00, 休憩12:00 ~ 13:00）として一括入力します。",
			Attachments: []slack.Attachment{
				slack.Attachment{
					CallbackID: callbackIDAttendanceButton,
					Actions: []slack.AttachmentAction{
						slack.AttachmentAction{
							Name:  actionTypeBulkMonth,
							Value: actionTypeBulkMonth,
							Text:  "一括入力する",
							Style: "primary",
							Type:  "button",
							Confirm: &slack.ConfirmationField{
								Text:        "本当に一括入力しますか？",
								OkText:      "はい",
								DismissText: "いいえ",
							},
						},
					},
				},
			},
		}, nil	
	}
	if timeTable.IsLeaving() {
		return &slack.Msg{
			Text: "既に退勤済です。打刻修正は <https://" + ctx.TeamSpiritHost + "|TeamSpirit> で行なってください。",
			Attachments: []slack.Attachment{
				slack.Attachment{
					CallbackID: callbackIDAttendanceButton,
					Actions: []slack.AttachmentAction{
						slack.AttachmentAction{
							Name:  actionTypeReset,
							Value: actionTypeReset,
							Text:  "リセットする",
							Style: "danger",
							Type:  "button",
							Confirm: &slack.ConfirmationField{
								Text:        "本当に本日の勤怠をリセットしますか？",
								OkText:      "はい",
								DismissText: "いいえ",
							},
						},
					},
				},
			},
		}, nil
	}
	if timeTable.IsHoliday != nil && *timeTable.IsHoliday == true {
		return &slack.Msg{
			Text: "本日は休日です :sunny:",
		}, nil
	}
	if timeTable.IsResting() {
		return &slack.Msg{
			Attachments: []slack.Attachment{
				slack.Attachment{
					CallbackID: callbackIDAttendanceButton,
					Actions: []slack.AttachmentAction{
						slack.AttachmentAction{
							Name:  actionTypeUnrest,
							Value: actionTypeUnrest,
							Text:  "休憩を終了する",
							Style: "default",
							Type:  "select",
							Options: []slack.AttachmentActionOption{
								{
									Text: "12:00",
									Value: "12:00",
								},
								{
									Text: "12:30",
									Value: "12:30",
								},
								{
									Text: "13:00",
									Value: "13:00",
								},
								{
									Text: "13:30",
									Value: "13:30",
								},
								{
									Text: "14:00",
									Value: "14:00",
								},
								{
									Text: "14:30",
									Value: "14:30",
								},
								{
									Text: "15:00",
									Value: "15:00",
								},
								{
									Text: "15:30",
									Value: "15:30",
								},
								{
									Text: "16:00",
									Value: "16:00",
								},
							},
							Confirm: &slack.ConfirmationField{
								Text:        "選択した時刻で休憩を終了しますか？",
								OkText:      "はい",
								DismissText: "いいえ",
							},
						},
						slack.AttachmentAction{
							Name:  actionTypeReset,
							Value: actionTypeReset,
							Text:  "リセットする",
							Style: "danger",
							Type:  "button",
							Confirm: &slack.ConfirmationField{
								Text:        "本当に本日の勤怠をリセットしますか？",
								OkText:      "はい",
								DismissText: "いいえ",
							},
						},
					},
				},
			},
		}, nil
	}
	if timeTable.IsAttending() {
		return &slack.Msg{
			Attachments: []slack.Attachment{
				slack.Attachment{
					CallbackID: callbackIDAttendanceButton,
					Actions: []slack.AttachmentAction{
						slack.AttachmentAction{
							Name:  actionTypeRest,
							Value: actionTypeRest,
							Text:  "休憩を開始する",
							Style: "default",
							Type:  "select",
							Options: []slack.AttachmentActionOption{
								{
									Text: "11:00",
									Value: "11:00",
								},
								{
									Text: "11:30",
									Value: "11:30",
								},
								{
									Text: "12:00",
									Value: "12:00",
								},
								{
									Text: "12:30",
									Value: "12:30",
								},
								{
									Text: "13:00",
									Value: "13:00",
								},
								{
									Text: "13:30",
									Value: "13:30",
								},
								{
									Text: "14:00",
									Value: "14:00",
								},
								{
									Text: "14:30",
									Value: "14:30",
								},
								{
									Text: "15:00",
									Value: "15:00",
								},
							},
							Confirm: &slack.ConfirmationField{
								Text:        "選択した時刻で休憩を開始しますか？",
								OkText:      "はい",
								DismissText: "いいえ",
							},
						},
						slack.AttachmentAction{
							Name:  actionTypeLeave,
							Value: actionTypeLeave,
							Text:  "退勤する",
							Style: "danger",
							Type:  "select",
							Options: []slack.AttachmentActionOption{
								{
									Text: "16:00",
									Value: "16:00",
								},
								{
									Text: "16:30",
									Value: "16:30",
								},
								{
									Text: "17:00",
									Value: "17:00",
								},
								{
									Text: "17:30",
									Value: "17:30",
								},
								{
									Text: "18:00",
									Value: "18:00",
								},
								{
									Text: "18:30",
									Value: "18:30",
								},
								{
									Text: "19:00",
									Value: "19:00",
								},
								{
									Text: "19:30",
									Value: "19:30",
								},
								{
									Text: "20:00",
									Value: "20:00",
								},
								{
									Text: "20:30",
									Value: "20:30",
								},
								{
									Text: "21:00",
									Value: "21:00",
								},
							},
							Confirm: &slack.ConfirmationField{
								Text:        "選択した時刻で退勤しますか？",
								OkText:      "はい",
								DismissText: "いいえ",
							},
						},
						slack.AttachmentAction{
							Name:  actionTypeReset,
							Value: actionTypeReset,
							Text:  "リセットする",
							Style: "danger",
							Type:  "button",
							Confirm: &slack.ConfirmationField{
								Text:        "本当に本日の勤怠をリセットしますか？",
								OkText:      "はい",
								DismissText: "いいえ",
							},
						},
					},
				},
			},
		}, nil
	}
	return &slack.Msg{
		Attachments: []slack.Attachment{
			slack.Attachment{
				CallbackID: callbackIDAttendanceButton,
				Actions: []slack.AttachmentAction{
					slack.AttachmentAction{
						Name:  actionTypeAttend,
						Value: actionTypeAttend,
						Text:  "出勤する",
						Style: "primary",
						Type:  "select",
						Options: []slack.AttachmentActionOption{
							{
								Text: "08:30",
								Value: "08:30",
							},
							{
								Text: "09:00",
								Value: "09:00",
							},
							{
								Text: "09:30",
								Value: "09:30",
							},
							{
								Text: "10:00",
								Value: "10:00",
							},
							{
								Text: "10:30",
								Value: "10:30",
							},
							{
								Text: "11:00",
								Value: "11:00",
							},
						},
						Confirm: &slack.ConfirmationField{
							Text:        "選択した時刻で出勤しますか？",
							OkText:      "はい",
							DismissText: "いいえ",
						},
					},
				},
			},
		},
	}, nil

}
