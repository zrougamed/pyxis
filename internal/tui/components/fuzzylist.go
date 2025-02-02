// Package components provides reusable Bubble Tea sub-models.
package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zrougamed/pyxis/internal/clipboard"
	"github.com/zrougamed/pyxis/internal/tui/styles"
	"github.com/zrougamed/pyxis/pkg/fuzzy"
)

// FuzzyListItem represents a single row in the list.
type FuzzyListItem struct {
	// ID is a stable identifier returned on selection.
	ID string
	// Name is the primary text (used for fuzzy matching).
	Name string
	// Detail is secondary text rendered after the name.
	Detail string
}

// FuzzyListSelection is the message emitted when the user presses Enter.
type FuzzyListSelection struct {
	Index int
	Item  FuzzyListItem
}

// FuzzyListCopy is the message emitted when the user presses 'c'.
type FuzzyListCopy struct {
	Text string
}

// FuzzyList is a scrollable, fuzzy-filterable list component.
type FuzzyList struct {
	Title  string
	Items  []FuzzyListItem
	Footer string // optional extra line below the list (e.g. filter indicator)

	search     textinput.Model
	searchMode bool
	cursor     int
	offset     int
	filtered   []int // indices into Items
	height     int
	width      int
	copied     bool
}

// NewFuzzyList creates a FuzzyList with the given title.
func NewFuzzyList(title string) FuzzyList {
	ti := textinput.New()
	ti.Placeholder = "Type to search..."
	ti.CharLimit = 100
	ti.Width = 40

	return FuzzyList{
		Title:  title,
		search: ti,
	}
}

// SetItems replaces the item list and resets the filter.
func (fl *FuzzyList) SetItems(items []FuzzyListItem) {
	fl.Items = items
	fl.filtered = makeRange(len(items))
	fl.cursor = 0
	fl.offset = 0
	fl.searchMode = false
	fl.search.SetValue("")
	fl.search.Blur()
}

// SetFilter applies a search query without entering interactive search mode.
func (fl *FuzzyList) SetFilter(query string) {
	fl.search.SetValue(query)
	fl.applyFilter()
	fl.cursor = 0
	fl.offset = 0
}

// SetSize updates the available viewport dimensions.
func (fl *FuzzyList) SetSize(width, height int) {
	fl.width = width
	fl.height = height
}

// SelectedItem returns the currently highlighted item, or nil.
func (fl *FuzzyList) SelectedItem() *FuzzyListItem {
	if fl.cursor >= len(fl.filtered) {
		return nil
	}
	idx := fl.filtered[fl.cursor]
	if idx >= len(fl.Items) {
		return nil
	}
	return &fl.Items[idx]
}

// IsSearching reports whether the search input is active.
func (fl *FuzzyList) IsSearching() bool {
	return fl.searchMode
}

// Reset clears all state.
func (fl *FuzzyList) Reset() {
	fl.Items = nil
	fl.filtered = nil
	fl.cursor = 0
	fl.offset = 0
	fl.searchMode = false
	fl.search.SetValue("")
	fl.search.Blur()
	fl.copied = false
}

// Update processes key and resize messages. Returns itself plus any command.
// Selection and copy are signalled via FuzzyListSelection / FuzzyListCopy messages.
func (fl FuzzyList) Update(msg tea.Msg) (FuzzyList, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		fl.width = msg.Width
		fl.height = msg.Height
		return fl, nil

	case tea.KeyMsg:
		return fl.handleKey(msg)
	}

	// Forward to textinput when searching.
	if fl.searchMode {
		var cmd tea.Cmd
		fl.search, cmd = fl.search.Update(msg)
		fl.applyFilter()
		return fl, cmd
	}

	return fl, nil
}

func (fl FuzzyList) handleKey(msg tea.KeyMsg) (FuzzyList, tea.Cmd) {
	key := msg.String()

	// While searching, only Enter / Esc / arrows escape.
	if fl.searchMode {
		switch key {
		case "enter":
			fl.searchMode = false
			fl.search.Blur()
			if item := fl.SelectedItem(); item != nil {
				return fl, func() tea.Msg {
					return FuzzyListSelection{Index: fl.filtered[fl.cursor], Item: *item}
				}
			}
			return fl, nil
		case "esc":
			fl.searchMode = false
			fl.search.Blur()
			fl.search.SetValue("")
			fl.applyFilter()
			return fl, nil
		case "up", "k":
			fl.moveUp()
			return fl, nil
		case "down", "j":
			fl.moveDown()
			return fl, nil
		default:
			var cmd tea.Cmd
			fl.search, cmd = fl.search.Update(msg)
			fl.applyFilter()
			return fl, cmd
		}
	}

	switch key {
	case "/":
		fl.searchMode = true
		fl.search.Focus()
		return fl, textinput.Blink
	case "up", "k":
		fl.moveUp()
	case "down", "j":
		fl.moveDown()
	case "enter":
		if item := fl.SelectedItem(); item != nil {
			return fl, func() tea.Msg {
				return FuzzyListSelection{Index: fl.filtered[fl.cursor], Item: *item}
			}
		}
	case "c":
		text := fl.copyableText()
		if text != "" {
			_ = clipboard.Copy(text)
			fl.copied = true
			return fl, func() tea.Msg { return FuzzyListCopy{Text: text} }
		}
	}

	return fl, nil
}

func (fl *FuzzyList) moveUp() {
	if fl.cursor > 0 {
		fl.cursor--
		fl.adjustScroll()
	}
}

func (fl *FuzzyList) moveDown() {
	if fl.cursor < len(fl.filtered)-1 {
		fl.cursor++
		fl.adjustScroll()
	}
}

func (fl *FuzzyList) adjustScroll() {
	vis := fl.visibleRows()
	if fl.cursor < fl.offset {
		fl.offset = fl.cursor
	}
	if fl.cursor >= fl.offset+vis {
		fl.offset = fl.cursor - vis + 1
	}
}

func (fl FuzzyList) visibleRows() int {
	return max(1, fl.height-8)
}

func (fl *FuzzyList) applyFilter() {
	query := fl.search.Value()
	if query == "" {
		fl.filtered = makeRange(len(fl.Items))
		fl.cursor = 0
		fl.offset = 0
		return
	}

	names := make([]string, len(fl.Items))
	for i, item := range fl.Items {
		names[i] = item.Name
	}

	matches := fuzzy.Find(query, names)
	fl.filtered = make([]int, len(matches))
	for i, m := range matches {
		fl.filtered[i] = m.Index
	}
	fl.cursor = 0
	fl.offset = 0
}

func (fl FuzzyList) copyableText() string {
	item := fl.SelectedItem()
	if item == nil {
		return ""
	}
	if item.Detail != "" {
		return item.Name + item.Detail
	}
	return item.Name
}

// View renders the list component.
func (fl FuzzyList) View() string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(styles.Subtitle.Render("  " + fl.Title))
	sb.WriteString("\n")

	// Search bar.
	if fl.searchMode {
		sb.WriteString(styles.InputField.Render("🔍 " + fl.search.View()))
	} else {
		sb.WriteString(styles.MutedText.Render("  Press / to search"))
	}
	sb.WriteString("\n")

	// Optional footer (e.g. filter indicator).
	if fl.Footer != "" {
		sb.WriteString(styles.MutedText.Render("  " + fl.Footer))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	if len(fl.filtered) == 0 {
		sb.WriteString(styles.MutedText.Render("  No results found."))
		sb.WriteString("\n")
		return sb.String()
	}

	vis := fl.visibleRows()
	end := min(fl.offset+vis, len(fl.filtered))

	for i := fl.offset; i < end; i++ {
		idx := fl.filtered[i]
		cursor := "  "
		nameStyle := styles.NormalItem
		if i == fl.cursor {
			cursor = styles.SelectedItem.Render("▸ ")
			nameStyle = styles.SelectedItem
		}

		name := ""
		detail := ""
		if idx < len(fl.Items) {
			name = fl.Items[idx].Name
			detail = fl.Items[idx].Detail
		}

		sb.WriteString(fmt.Sprintf("%s%s%s\n",
			cursor,
			nameStyle.Render(name),
			styles.MutedText.Render(detail),
		))
	}

	// Scroll indicator.
	if len(fl.filtered) > vis {
		sb.WriteString(styles.MutedText.Render(fmt.Sprintf("\n  Showing %d–%d of %d",
			fl.offset+1, end, len(fl.filtered))))
	}

	return sb.String()
}

func makeRange(n int) []int {
	r := make([]int, n)
	for i := range r {
		r[i] = i
	}
	return r
}
