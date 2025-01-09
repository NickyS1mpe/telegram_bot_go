package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"config"
)

var (
	docStyle        = lipgloss.NewStyle().Margin(1, 2)
	quitTextStyle   = lipgloss.NewStyle().Margin(1, 0, 2, 4)
	paginationStyle = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	bot             *tgbotapi.BotAPI
	updates         tgbotapi.UpdatesChannel
	bot_token       = config.BotToken
	chatIDs         = config.Ids
	chatId          int64
)

type model struct {
	viewport    viewport.Model
	messages    []string
	textarea    textarea.Model
	senderStyle lipgloss.Style
	err         error
	sub         chan telegramUpdate
	list        list.Model
	choice      string
	id          string
}

type telegramUpdate struct {
	Content string
	Name    string
}

type item struct {
	title string
	id    string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.id }
func (i item) FilterValue() string { return i.title }

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

	vp := viewport.New(80, 5)
	vp.SetContent(`Welcome to the chat room!
Type a message and press Enter to send.`)

	ta.KeyMap.InsertNewline.SetEnabled(false)

	items := []list.Item{
		// item{title: "Raspberry Pi's", id: "2"},
	}

	for _, id := range chatIDs {
		items = append(items, item{title: id.Title, id: fmt.Sprintf("%d", id.Id)})
	}

	m := model{
		textarea:    ta,
		messages:    []string{},
		viewport:    vp,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:         nil,
		sub:         make(chan telegramUpdate),
		list:        list.New(items, list.NewDefaultDelegate(), 0, 0),
		choice:      "",
		id:          "",
	}

	m.list.Title = "Select a chatroom"
	m.list.Styles.PaginationStyle = paginationStyle

	return m
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
		if m.choice == "" {
			h, v := docStyle.GetFrameSize()
			m.list.SetSize(msg.Width-h, msg.Height-v)
		} else {
			m.viewport.Width = msg.Width
			m.textarea.SetWidth(msg.Width)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// Quit.
			fmt.Println(m.textarea.Value())
			return m, tea.Quit
		case "enter":
			if m.choice != "" {
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
				telegramBotSendText(chatId, v)
			} else {
				i, ok := m.list.SelectedItem().(item)
				if ok {
					m.choice = i.title
					m.id = i.id
					id, err := strconv.ParseInt(i.id, 10, 64)
					if err != nil {
						fmt.Println("Error:", err)
						return m, nil
					}
					chatId = id
				}
			}
			return m, nil

		default:
			// Send all other keypresses to the textarea.
			var cmd tea.Cmd
			if m.choice == "" {
				m.list, cmd = m.list.Update(msg)
			} else {
				m.textarea, cmd = m.textarea.Update(msg)
			}
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

		var cmd tea.Cmd
		if m.choice == "" {
			m.list, cmd = m.list.Update(msg)
		} else {
			m.textarea, cmd = m.textarea.Update(msg)
		}
		return m, cmd
	}

}

func (m model) View() string {
	switch m.choice {
	case "":
		return docStyle.Render(m.list.View())
	default:
		return fmt.Sprintf(
			"%s\n\n%s",
			m.viewport.View(),
			m.textarea.View(),
		) + "\n\n"
	}
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
			if update.Message != nil && update.Message.Chat.ID == chatId {
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
