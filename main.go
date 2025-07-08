package main

import (
	"log"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

var sessions = make(map[int64]time.Time)

func main() {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Panic(err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, _ := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			handleMessage(bot, update.Message)
		}

		if update.CallbackQuery != nil {
			handleCallback(bot, update.CallbackQuery)
		}
	}
}

func handleMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	// Если пришло любое сообщение или /start, покажем кнопку "Начать тренировку"
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏋️ Начать тренировку", "start_workout"),
		),
	)
	m := tgbotapi.NewMessage(msg.Chat.ID, "Нажми кнопку, чтобы начать тренировку")
	m.ReplyMarkup = keyboard
	bot.Send(m)
}

func handleCallback(bot *tgbotapi.BotAPI, callback *tgbotapi.CallbackQuery) {
	// Обязательно отвечаем Telegram, чтобы кнопка не подвисала
	bot.AnswerCallbackQuery(tgbotapi.NewCallback(callback.ID, ""))

	userID := int64(callback.From.ID)
	chatID := callback.Message.Chat.ID

	switch callback.Data {
	case "start_workout":
		sessions[userID] = time.Now()

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Закончить тренировку", "stop_workout"),
			),
		)
		// Редактируем сообщение с кнопкой, меняем текст и кнопку
		edit := tgbotapi.NewEditMessageText(chatID, callback.Message.MessageID, "Тренировка началась!")
		edit.ReplyMarkup = &keyboard
		bot.Send(edit)

	case "stop_workout":
		startTime, ok := sessions[userID]
		if !ok {
			bot.Send(tgbotapi.NewMessage(chatID, "Сначала начни тренировку."))
			return
		}

		duration := time.Since(startTime)
		delete(sessions, userID)

		text := "Тренировка завершена! Длительность: " + duration.Truncate(time.Second).String()

		// Удаляем старое сообщение с кнопкой "Закончить тренировку"
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, callback.Message.MessageID)
		bot.Send(deleteMsg)

		// Отправляем новое сообщение с итогом тренировки (без кнопок)
		bot.Send(tgbotapi.NewMessage(chatID, text))

		// TODO: сохранение в БД
	}
}
