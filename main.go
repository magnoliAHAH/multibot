package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	_ "github.com/lib/pq"
)

var (
	sessions = make(map[int64]time.Time)
	db       *sql.DB
)

func main() {
	var err error
	db, err = connectDB()
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	err = createTables()
	if err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

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

func connectDB() (*sql.DB, error) {
	host := os.Getenv("POSTGRES_HOST")
	port := os.Getenv("POSTGRES_PORT")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	dbname := os.Getenv("POSTGRES_DB")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	return sql.Open("postgres", connStr)
}

func createTables() error {
	userTable := `
	CREATE TABLE IF NOT EXISTS users (
		id BIGINT PRIMARY KEY,
		username TEXT,
		first_name TEXT,
		last_name TEXT
	);`

	workoutTable := `
	CREATE TABLE IF NOT EXISTS workouts (
		id SERIAL PRIMARY KEY,
		user_id BIGINT REFERENCES users(id),
		start_time TIMESTAMP NOT NULL,
		duration INTERVAL NOT NULL
	);`

	_, err := db.Exec(userTable)
	if err != nil {
		return err
	}

	_, err = db.Exec(workoutTable)
	return err
}

func saveUser(userID int64, username, firstName, lastName string) error {
	_, err := db.Exec(`
		INSERT INTO users (id, username, first_name, last_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET username = EXCLUDED.username,
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name
	`, userID, username, firstName, lastName)
	return err
}

func saveWorkout(userID int64, start time.Time, duration time.Duration) error {
	// Преобразуем duration в строку — формат '1h2m3s', понятный PostgreSQL
	durationStr := duration.String()
	_, err := db.Exec(`
		INSERT INTO workouts (user_id, start_time, duration)
		VALUES ($1, $2, $3::interval)
	`, userID, start, durationStr)
	return err
}

func getTotalWorkoutToday(userID int64) (time.Duration, error) {
	startOfDay := time.Now().Truncate(24 * time.Hour)

	var seconds float64
	row := db.QueryRow(`
		SELECT EXTRACT(EPOCH FROM COALESCE(SUM(duration), INTERVAL '0'))
		FROM workouts
		WHERE user_id = $1 AND start_time >= $2
	`, userID, startOfDay)

	err := row.Scan(&seconds)
	if err != nil {
		return 0, err
	}

	return time.Duration(seconds * float64(time.Second)), nil
}

func handleMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	userID := int64(msg.From.ID)
	// Сохраняем пользователя при любом сообщении
	err := saveUser(userID, msg.From.UserName, msg.From.FirstName, msg.From.LastName)
	if err != nil {
		log.Println("Error saving user:", err)
	}

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

		// Сохраняем тренировку в БД
		err := saveWorkout(userID, startTime, duration)
		if err != nil {
			log.Println("Error saving workout:", err)
		}

		// Получаем суммарное время тренировок за сегодня
		total, err := getTotalWorkoutToday(userID)
		if err != nil {
			log.Println("Error getting total workout time:", err)
		}

		text := fmt.Sprintf("Тренировка завершена! Длительность: %s\nОбщее время сегодня: %s",
			formatDuration(duration),
			formatDuration(total))

		// Удаляем сообщение с кнопкой
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, callback.Message.MessageID)
		bot.Send(deleteMsg)

		// Отправляем сообщение с результатом
		bot.Send(tgbotapi.NewMessage(chatID, text))
	}
}

func formatDuration(d time.Duration) string {
	// Округляем до ближайшей секунды
	seconds := int(math.Round(d.Seconds()))

	minutes := seconds / 60
	seconds = seconds % 60

	if minutes > 0 {
		return fmt.Sprintf("%d мин %d сек", minutes, seconds)
	}
	return fmt.Sprintf("%d сек", seconds)
}
