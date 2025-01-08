package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var bot *tgbotapi.BotAPI
var updates tgbotapi.UpdatesChannel

func main() {
	var err error
	bot, err = tgbotapi.NewBotAPI(bot_token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = false
	getChannel()

	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Oof: %v\n", err)
	}
}

type model struct {
	viewport    viewport.Model
	messages    []string
	textarea    textarea.Model
	senderStyle lipgloss.Style
	err         error
	sub         chan telegramUpdate
}

type telegramUpdate struct {
	Content string
	Name    string
}

func initialModel() model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()

	ta.Prompt = "┃ "
	ta.CharLimit = 280

	ta.SetWidth(30)
	ta.SetHeight(3)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	ta.ShowLineNumbers = false

	vp := viewport.New(30, 5)
	vp.SetContent(`Welcome to the chat room!
Type a message and press Enter to send.`)

	ta.KeyMap.InsertNewline.SetEnabled(false)

	return model{
		textarea:    ta,
		messages:    []string{},
		viewport:    vp,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:         nil,
		sub:         make(chan telegramUpdate),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		fetchHistory(m.sub),
		waitForActivity(m.sub),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.textarea.SetWidth(msg.Width)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			// Quit.
			fmt.Println(m.textarea.Value())
			return m, tea.Quit
		case "enter":
			v := m.textarea.Value()

			if v == "" {
				// Don't send empty messages.
				m.messages = append(m.messages, m.senderStyle.Render("Server: Don't send empty messages."))
				m.viewport.SetContent(strings.Join(m.messages, "\n"))
				m.textarea.Reset()
				m.viewport.GotoBottom()
				return m, nil
			}

			m.messages = append(m.messages, m.senderStyle.Render("You: ")+v)
			m.viewport.SetContent(strings.Join(m.messages, "\n"))
			m.textarea.Reset()
			m.viewport.GotoBottom()
			telegramBotSendText(chatID, v)
			return m, nil

		default:
			// Send all other keypresses to the textarea.
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		}

	case cursor.BlinkMsg:
		// Textarea should also process cursor blinks.
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd

	case telegramUpdate:
		m.messages = append(m.messages, m.senderStyle.Render(msg.Name+": ")+msg.Content)
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()
		return m, waitForActivity(m.sub)

	default:

		return m, nil
	}

}

func (m model) View() string {
	return fmt.Sprintf(
		"%s\n\n%s",
		m.viewport.View(),
		m.textarea.View(),
	) + "\n\n"
}

func telegramBotSendText(chatID int64, botMsg string) {
	msg := tgbotapi.NewMessage(chatID, botMsg)

	bot.Send(msg)
}

func getChannel() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates = bot.GetUpdatesChan(u)
}

func fetchHistory(sub chan telegramUpdate) tea.Cmd {
	return func() tea.Msg {
		for update := range updates {
			if update.Message != nil && update.Message.Chat.ID == chatID {
				sub <- telegramUpdate{update.Message.Text, update.Message.From.FirstName + " " + update.Message.From.LastName}
			}
		}
		return nil
	}
}

func waitForActivity(sub chan telegramUpdate) tea.Cmd {
	return func() tea.Msg {
		return telegramUpdate(<-sub)
	}
}
