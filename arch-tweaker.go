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

type state struct {
	Selected map[string]bool `json:"selected"`
	Cursor   int             `json:"cursor"`
}

type tweak struct {
	id       string
	title    string
	selected bool
	commands [][]string
}

func (m model) selectedMap() map[string]bool {
	sel := make(map[string]bool, len(m.tweaks))
	for _, t := range m.tweaks {
		sel[t.id] = t.selected
	}
	return sel
}

func statePath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "arch-tweaker", "state.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "arch-tweaker-state.json"
	}
	return filepath.Join(home, ".config", "arch-tweaker", "state.json")
}

func loadState() state {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return state{Selected: map[string]bool{}, Cursor: 0}
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return state{Selected: map[string]bool{}, Cursor: 0}
	}
	if s.Selected == nil {
		s.Selected = map[string]bool{}
	}
	return s
}

func saveState(s state) {
	path := statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return
	}
}

type model struct {
	tweaks   []tweak
	cursor   int
	executed bool
}

func pick(saved map[string]bool, id string, def bool) bool {
	if v, ok := saved[id]; ok {
		return v
	}
	return def
}

func initialModel() model {
	saved := loadState()
	tweaks := []tweak{
		{
			id:       "update",
			title:    "Обновление системы (pacman -Syu)",
			selected: pick(saved.Selected, "update", true),
			commands: [][]string{{"sudo", "pacman", "-Syu", "--noconfirm"}},
		},
		{
			id:       "cache-clean",
			title:    "Очистка кэша пакетов (удалить старые версии)",
			selected: pick(saved.Selected, "cache-clean", false),
			commands: [][]string{{"sudo", "pacman", "-Sc", "--noconfirm"}},
		},
		{
			id:       "orphans",
			title:    "Удаление неиспользуемых зависимостей (сирот)",
			selected: pick(saved.Selected, "orphans", false),
			commands: [][]string{{"bash", "-c", "sudo pacman -Qdtq | xargs -r sudo pacman -Rns --noconfirm"}},
		},
		{
			id:       "ssd-trim",
			title:    "Оптимизация SSD (включение fstrim.timer)",
			selected: pick(saved.Selected, "ssd-trim", true),
			commands: [][]string{
				{"sudo", "systemctl", "enable", "fstrim.timer"},
				{"sudo", "systemctl", "start", "fstrim.timer"},
			},
		},
		{
			id:       "mirrors",
			title:    "Оптимизация зеркал (установка и запуск reflector)",
			selected: pick(saved.Selected, "mirrors", false),
			commands: [][]string{
				{"sudo", "pacman", "-S", "reflector", "--noconfirm", "--needed"},
				{"sudo", "reflector", "--latest", "5", "--protocol", "https", "--sort", "rate", "--save", "/etc/pacman.d/mirrorlist"},
			},
		},
		{
			id:       "yay",
			title:    "Установка помощника yay (bin)",
			selected: pick(saved.Selected, "yay", false),
			commands: [][]string{
				{"sudo", "pacman", "-S", "--needed", "git", "base-devel", "--noconfirm"},
				{"bash", "-c", "cd /tmp && git clone https://aur.archlinux.org/yay-bin.git && cd yay-bin && makepkg -si --noconfirm"},
			},
		},
	}
	m := model{tweaks: tweaks}
	if saved.Cursor >= 0 && saved.Cursor < len(m.tweaks) {
		m.cursor = saved.Cursor
	} else {
		m.cursor = 0
	}
	return m
}

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
	s += "[Пробел/Enter] — выбрать/отменить. [X] — запустить. [Q] — выход.\n"
	s += "(Выбор сохраняется между запусками)\n\n"

	for i, tweak := range m.tweaks {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		checked := " "
		if tweak.selected {
			checked = "X"
		}

		s += fmt.Sprintf("%s [%s] %s\n", cursor, checked, tweak.title)
	}

	return s
}

func main() {
	p := tea.NewProgram(initialModel())
	m, err := p.Run()
	if err != nil {
		fmt.Printf("Ошибка запуска интерфейса: %v\n", err)
		os.Exit(1)
	}

	finalModel := m.(model)

	saveState(state{
		Selected: finalModel.selectedMap(),
		Cursor:   finalModel.cursor,
	})

	if finalModel.executed {
		fmt.Println("\n[+] Запуск выбранных оптимизаций...")
		for _, t := range finalModel.tweaks {
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
					break
				}
			}
		}
		fmt.Println("\n[Все готово!] Система оптимизирована.")
	} else {
		fmt.Println("\nВыход без изменений.")
	}
}
