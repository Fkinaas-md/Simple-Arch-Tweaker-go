package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Настройки и состояние ---

type state struct {
	Selected map[string]bool `json:"selected"`
	Cursor   int             `json:"cursor"`
}

func statePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir() // Фолбэк, если конфиг-директория недоступна
	}
	return filepath.Join(dir, "arch-tweaker", "state.json")
}

func loadState() state {
	s := state{Selected: make(map[string]bool)}
	data, err := os.ReadFile(statePath())
	if err == nil {
		json.Unmarshal(data, &s)
	}
	if s.Selected == nil {
		s.Selected = make(map[string]bool)
	}
	return s
}

func saveState(m model) {
	s := state{
		Selected: make(map[string]bool),
		Cursor:   m.cursor,
	}
	for _, t := range m.tweaks {
		s.Selected[t.id] = t.selected
	}

	path := statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
		if data, err := json.MarshalIndent(s, "", "  "); err == nil {
			os.WriteFile(path, data, 0644)
		}
	}
}

// --- Структуры данных ---

type tweak struct {
	id       string
	title    string
	selected bool
	commands [][]string
}

type model struct {
	tweaks   []tweak
	cursor   int
	executed bool
}

func initialModel() model {
	saved := loadState()
	
	// Вспомогательная функция для загрузки сохраненного выбора
	pick := func(id string, def bool) bool {
		if v, ok := saved.Selected[id]; ok {
			return v
		}
		return def
	}

	tweaks := []tweak{
		{
			id:       "update",
			title:    "Обновление системы (pacman -Syu)",
			selected: pick("update", true),
			commands: [][]string{{"sudo", "pacman", "-Syu", "--noconfirm"}},
		},
		{
			id:       "cache-clean",
			title:    "Очистка кэша пакетов (удалить старые версии)",
			selected: pick("cache-clean", false),
			commands: [][]string{{"sudo", "pacman", "-Sc", "--noconfirm"}},
		},
		{
			id:       "orphans",
			title:    "Удаление неиспользуемых зависимостей (сирот)",
			selected: pick("orphans", false),
			commands: [][]string{{"bash", "-c", "sudo pacman -Qdtq | xargs -r sudo pacman -Rns --noconfirm"}},
		},
		{
			id:       "ssd-trim",
			title:    "Оптимизация SSD (включение fstrim.timer)",
			selected: pick("ssd-trim", true),
			commands: [][]string{
				{"sudo", "systemctl", "enable", "fstrim.timer"},
				{"sudo", "systemctl", "start", "fstrim.timer"},
			},
		},
		{
			id:       "mirrors",
			title:    "Оптимизация зеркал (установка и запуск reflector)",
			selected: pick("mirrors", false),
			commands: [][]string{
				{"sudo", "pacman", "-S", "reflector", "--noconfirm", "--needed"},
				{"sudo", "reflector", "--latest", "5", "--protocol", "https", "--sort", "rate", "--save", "/etc/pacman.d/mirrorlist"},
			},
		},
		{
			id:       "yay",
			title:    "Установка помощника yay (bin)",
			selected: pick("yay", false),
			commands: [][]string{
				{"sudo", "pacman", "-S", "--needed", "git", "base-devel", "--noconfirm"},
				{"bash", "-c", "cd /tmp && git clone https://aur.archlinux.org/yay-bin.git && cd yay-bin && makepkg -si --noconfirm"},
			},
		},
	}

	cursor := saved.Cursor
	if cursor < 0 || cursor >= len(tweaks) {
		cursor = 0
	}

	return model{tweaks: tweaks, cursor: cursor}
}

// --- Логика Bubble Tea ---

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.tweaks)-1 {
				m.cursor++
			}
		case " ", "enter":
			m.tweaks[m.cursor].selected = !m.tweaks[m.cursor].selected
		case "x":
			m.executed = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.executed {
		return "Выход из интерфейса настройки. Начинаем применение твиков...\n"
	}

	s := "--- ARCH LINUX TERMINAL TWEAKER ---\n\n"
	s += "Используйте стрелочки или j/k для навигации.\n"
	s += "[Пробел/Enter] — выбрать/отменить. [X] — запустить. [Q] — выход.\n\n"

	for i, t := range m.tweaks {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		checked := " "
		if t.selected {
			checked = "X"
		}
		s += fmt.Sprintf("%s [%s] %s\n", cursor, checked, t.title)
	}

	return s
}

// --- Выполнение ---

func main() {
	p := tea.NewProgram(initialModel())
	finalModelRaw, err := p.Run()
	if err != nil {
		fmt.Printf("Ошибка запуска интерфейса: %v\n", err)
		os.Exit(1)
	}

	m := finalModelRaw.(model)
	saveState(m) // Сохраняем выбор и положение курсора

	if !m.executed {
		fmt.Println("\nВыход без изменений.")
		return
	}

	fmt.Println("\n[+] Запуск выбранных оптимизаций...")
	for _, t := range m.tweaks {
		if !t.selected {
			continue
		}
		
		fmt.Printf("\n--> Выполняется: %s\n", t.title)
		for _, cmdArgs := range t.commands {
			if len(cmdArgs) == 0 {
				continue
			}
			
			fmt.Printf("Запуск: %s\n", strings.Join(cmdArgs, " "))
			cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin

			if err := cmd.Run(); err != nil {
				fmt.Printf("[Ошибка] Не удалось выполнить: %v\n", err)
				break // Если команда упала, не выполняем следующие из этого твика
			}
		}
	}
	
	fmt.Println("\n[Все готово!] Система оптимизирована.")
}
