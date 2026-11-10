// Package install configures Claude Code, Codex and Antigravity CLI to call Sidecar on every prompt, with
// the user's consent, and reverses exactly what it changed. Existing settings are merged,
// never replaced; a backup is taken before the first change; and a manifest records every
// file and entry so uninstall removes only Sidecar's own entries.
package install

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/paths"
)

// BackupSuffix is appended to a settings file for its pre-install copy.
const BackupSuffix = ".sidecar-backup"

// hookTimeoutSeconds bounds a hook run. Sidecar answers within its own budget well before this.
const hookTimeoutSeconds = 10

// BinaryName is the installed executable name for this platform.
var BinaryName = map[bool]string{true: "sidecar.exe", false: "sidecar"}[runtime.GOOS == "windows"]

// Env holds every path the installer touches, so tests can point them at temp folders.
type Env struct {
	Source         string // the executable to install
	InstallDir     string
	ClaudeSettings string // Claude Code user settings (~/.claude/settings.json)
	CodexHooks     string // Codex user hooks (~/.codex/hooks.json)
	CodexConfig    string // Codex config (read-only here, for diagnostics)
	// AntigravityHooks is Antigravity CLI's global hooks file (~/.gemini/config/hooks.json).
	// Empty disables Antigravity entirely, so callers that do not set it never touch it.
	AntigravityHooks string
	Manifest         string
}

// Options controls Install.
type Options struct {
	Env    Env
	Agents []string  // "claude", "codex", "antigravity"; empty means every detected agent
	Yes    bool      // consent given in advance (--yes)
	In     io.Reader // where the consent answer is read from; nil means non-interactive
	Out    io.Writer
	// StatusLine also configures the Claude Code status line (opt-in, --statusline). An existing
	// status line from another tool is never replaced.
	StatusLine bool
	// ObserveTools also adds the tool-event hooks that `sidecar observe` receives (--observe-tools,
	// opt-in). They only record metadata; they never change a tool call.
	ObserveTools bool
}

// UninstallOptions controls Uninstall.
type UninstallOptions struct {
	Env Env
	Out io.Writer
}

type entry struct {
	Agent       string `json:"agent"`
	Event       string `json:"event,omitempty"`
	File        string `json:"file"`
	Command     string `json:"command"`
	FileCreated bool   `json:"file_created"`
}

// hookEvent is the client event an entry belongs to. Entries recorded before tool observation
// existed have no Event and belong to the prompt hook.
func (e entry) hookEvent() string {
	if e.Event != "" {
		return e.Event
	}
	return promptEvent(e.Agent)
}

// statusLineEntry records a status line this install added, so uninstall removes only it.
type statusLineEntry struct {
	File    string `json:"file"`
	Command string `json:"command"`
}

type manifest struct {
	Binary        string           `json:"binary,omitempty"`
	BinaryCreated bool             `json:"binary_created,omitempty"`
	Entries       []entry          `json:"entries"`
	StatusLine    *statusLineEntry `json:"statusline,omitempty"`
}

// DefaultEnv resolves the real locations for this user and platform.
func DefaultEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	src, err := os.Executable()
	if err != nil {
		return Env{}, err
	}
	if resolved, err := filepath.EvalSymlinks(src); err == nil {
		src = resolved
	}
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		claudeDir = filepath.Join(home, ".claude")
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	installDir := filepath.Join(home, ".local", "bin")
	if runtime.GOOS == "windows" {
		installDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "sidecar")
	}
	return Env{
		Source:           src,
		InstallDir:       installDir,
		ClaudeSettings:   filepath.Join(claudeDir, "settings.json"),
		CodexHooks:       filepath.Join(codexHome, "hooks.json"),
		CodexConfig:      filepath.Join(codexHome, "config.toml"),
		AntigravityHooks: filepath.Join(home, ".gemini", "config", "hooks.json"),
		Manifest:         paths.Manifest(),
	}, nil
}

// Detect lists agents that appear to be installed: their config folder exists or their
// command is on PATH.
func Detect(env Env) []string {
	var found []string
	if dirExists(filepath.Dir(env.ClaudeSettings)) || onPath("claude") {
		found = append(found, "claude")
	}
	if dirExists(filepath.Dir(env.CodexHooks)) || onPath("codex") {
		found = append(found, "codex")
	}
	if env.AntigravityHooks != "" && (dirExists(filepath.Dir(env.AntigravityHooks)) || onPath("agy")) {
		found = append(found, "antigravity")
	}
	return found
}

// Install asks for consent (unless Yes), installs the binary, and adds one hook per agent.
func Install(opts Options) ([]string, error) {
	agents := opts.Agents
	if len(agents) == 0 {
		agents = Detect(opts.Env)
	}
	if len(agents) == 0 {
		return nil, errors.New("no supported agent found (Claude Code, Codex or Antigravity CLI)")
	}
	plan := describe(opts.Env, agents, opts.StatusLine, opts.ObserveTools)
	for _, line := range plan {
		_, _ = fmt.Fprintln(opts.Out, "  "+line)
	}
	if !opts.Yes {
		if opts.In == nil {
			return nil, errors.New("this is not an interactive terminal; re-run with --yes to confirm the changes above")
		}
		_, _ = fmt.Fprint(opts.Out, "Aplicar estas alterações? [s/N] ")
		answer, _ := bufio.NewReader(opts.In).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "s", "sim", "y", "yes":
		default:
			_, _ = fmt.Fprintln(opts.Out, "Nada foi alterado.")
			return nil, nil
		}
	}

	m := loadManifest(opts.Env.Manifest)
	dst := filepath.Join(opts.Env.InstallDir, BinaryName)
	if !samePath(opts.Env.Source, dst) {
		created := !fileExists(dst)
		// Do not clobber an executable we cannot prove belongs to this install.
		// A missing/corrupt manifest must never turn an existing binary into ours.
		if !created && (m.Binary != dst || !m.BinaryCreated) {
			return nil, fmt.Errorf("refusing to overwrite unmanaged binary %s; move it or uninstall it first", dst)
		}
		if err := copyExecutable(opts.Env.Source, dst); err != nil {
			return nil, fmt.Errorf("installing binary: %w", err)
		}
		if m.Binary == "" || m.Binary == dst {
			m.Binary = dst
			m.BinaryCreated = m.BinaryCreated || created
		}
	}
	var done []string
	for _, agent := range agents {
		file := settingsFile(opts.Env, agent)
		created := !fileExists(file)
		if !created {
			if err := backupOnce(file); err != nil {
				return done, err
			}
		}
		for _, h := range hooksFor(dst, agent, opts.ObserveTools) {
			if err := editAgentHooks(agent, file, h.event, h.command, true); err != nil {
				return done, fmt.Errorf("%s: %w", file, err)
			}
			m.upsert(entry{Agent: agent, Event: h.event, File: file, Command: h.command, FileCreated: created || m.createdFor(file)})
		}
		done = append(done, fmt.Sprintf("%s: hook instalado em %s", agent, file))
		if names := toolEventNames(agent); opts.ObserveTools && len(names) > 0 {
			done = append(done, fmt.Sprintf("%s: observação de ferramentas (%s) instalada em %s", agent, strings.Join(names, ", "), file))
		}
	}
	if opts.StatusLine && slices.Contains(agents, "claude") {
		msg, err := installStatusLine(opts.Env, dst, &m)
		if err != nil {
			return done, err
		}
		done = append(done, msg)
	}
	if err := saveManifest(opts.Env.Manifest, m); err != nil {
		return done, err
	}
	return done, nil
}

// Uninstall removes exactly the hook entries, backups and binary that Install recorded.
func Uninstall(opts UninstallOptions) ([]string, error) {
	m, err := readManifest(opts.Env.Manifest)
	if err != nil {
		return nil, errors.New("nothing to uninstall: no installation record found")
	}
	var done []string
	// The status line goes first: the hook loop may delete a settings file that this step still edits.
	if m.StatusLine != nil {
		if err := editStatusLine(m.StatusLine.File, m.StatusLine.Command, true); err != nil {
			return done, fmt.Errorf("%s: %w", m.StatusLine.File, err)
		}
		done = append(done, "claude: statusLine do Sidecar removida de "+m.StatusLine.File)
	}
	for _, e := range m.Entries {
		if err := editAgentHooks(e.Agent, e.File, e.hookEvent(), e.Command, false); err != nil {
			return done, fmt.Errorf("%s: %w", e.File, err)
		}
		if e.FileCreated && isEmptySettings(e.File) {
			_ = os.Remove(e.File)
			_ = os.Remove(e.File + BackupSuffix)
			done = append(done, "removido "+e.File)
		} else {
			_ = os.Remove(e.File + BackupSuffix)
			done = append(done, fmt.Sprintf("%s: hook removido de %s", e.Agent, e.File))
		}
	}
	if m.BinaryCreated && m.Binary != "" {
		if err := os.Remove(m.Binary); err == nil {
			done = append(done, "removido "+m.Binary)
		}
	}
	if err := os.Remove(opts.Env.Manifest); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return done, err
	}
	return done, nil
}

// HookCommand is the command line an agent runs for a prompt.
func HookCommand(bin, agent string) string {
	return quoteBin(bin) + " hook --agent " + agent
}

// observeCommand is the command a tool-event hook runs: `sidecar observe` for that event.
func observeCommand(bin, agent, event string) string {
	return quoteBin(bin) + " observe --agent " + agent + " --event " + event
}

// promptEvent is the event that runs Sidecar's prompt hook for an agent.
func promptEvent(agent string) string {
	return adapter.CapabilitiesFor(adapter.Agent(agent)).PromptEvent
}

// toolEventNames lists the client event names of an agent's tool hooks, for messages.
func toolEventNames(agent string) []string {
	var names []string
	for _, hook := range adapter.CapabilitiesFor(adapter.Agent(agent)).ObservationHooks {
		names = append(names, hook.Name)
	}
	return names
}

// hookFor is one handler an install adds to an agent's hook file.
type hookFor struct{ event, command string }

// hooksFor lists the handlers an install adds for one agent: the prompt hook, and the tool
// observers when observeTools is set.
func hooksFor(bin, agent string, observeTools bool) []hookFor {
	capabilities := adapter.CapabilitiesFor(adapter.Agent(agent))
	if capabilities.PromptEvent == "" {
		return nil
	}
	out := []hookFor{{capabilities.PromptEvent, HookCommand(bin, agent)}}
	if observeTools {
		for _, hook := range capabilities.ObservationHooks {
			out = append(out, hookFor{hook.Name, observeCommand(bin, agent, hook.ObserveEvent)})
		}
	}
	return out
}

// statusLineCommand is the command Claude Code runs to render the status line.
func statusLineCommand(bin string) string {
	return quoteBin(bin) + " statusline"
}

func quoteBin(bin string) string {
	if runtime.GOOS == "windows" || strings.ContainsAny(bin, " \t\"'&()") {
		return `"` + strings.ReplaceAll(bin, `"`, `\"`) + `"`
	}
	return bin
}

// IsOurs reports whether a hook command was installed by Sidecar for an agent.
func IsOurs(command, agent string) bool {
	suffix := " hook --agent " + agent
	return strings.HasSuffix(command, suffix) && isSidecarBinary(strings.TrimSuffix(command, suffix))
}

// IsOursStatusLine reports whether a status line command was installed by Sidecar.
func IsOursStatusLine(command string) bool {
	suffix := " statusline"
	return strings.HasSuffix(command, suffix) && isSidecarBinary(strings.TrimSuffix(command, suffix))
}

func isSidecarBinary(quoted string) bool {
	bin := strings.Trim(quoted, `"`)
	name := bin[strings.LastIndexAny(bin, `/\`)+1:]
	return name == "sidecar" || name == "sidecar.exe"
}

func describe(env Env, agents []string, statusLine, observeTools bool) []string {
	lines := []string{fmt.Sprintf("Instalar o binário em %s", filepath.Join(env.InstallDir, BinaryName))}
	for _, a := range agents {
		line := fmt.Sprintf("%s: adicionar hook %s em %s", a, promptEvent(a), settingsFile(env, a))
		if a == "codex" {
			line += " (o Codex pedirá para revisar e aprovar o hook na primeira execução)"
		}
		lines = append(lines, line)
		if names := toolEventNames(a); observeTools && len(names) > 0 {
			lines = append(lines, fmt.Sprintf("%s: adicionar observação de ferramentas (%s) em %s; só registra metadados, nunca altera a chamada", a, strings.Join(names, ", "), settingsFile(env, a)))
		}
	}
	if statusLine && slices.Contains(agents, "claude") {
		file := settingsFile(env, "claude")
		obj, _ := readObject(file)
		if _, has := obj["statusLine"]; has && !ownStatusLine(obj["statusLine"]) {
			lines = append(lines, "claude: statusLine já existe e será preservada (não substituída); o snippet manual será exibido")
		} else {
			lines = append(lines, "claude: configurar statusLine do Sidecar (opt-in) em "+file)
		}
	}
	return lines
}

// installStatusLine adds the Sidecar status line when none exists. A status line from another
// tool is left in place and its manual snippet is returned instead.
func installStatusLine(env Env, bin string, m *manifest) (string, error) {
	file := settingsFile(env, "claude")
	obj, err := readObject(file)
	if err != nil {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	command := statusLineCommand(bin)
	if _, has := obj["statusLine"]; has && !ownStatusLine(obj["statusLine"]) {
		return "claude: statusLine existente preservada (não substituída); para usar a do Sidecar, troque manualmente por: " + command, nil
	}
	if !ownStatusLine(obj["statusLine"]) {
		if fileExists(file) {
			if err := backupOnce(file); err != nil {
				return "", err
			}
		}
		obj["statusLine"] = map[string]any{"type": "command", "command": command}
		if err := writeObject(file, obj); err != nil {
			return "", fmt.Errorf("%s: %w", file, err)
		}
	}
	m.StatusLine = &statusLineEntry{File: file, Command: command}
	return "claude: statusLine do Sidecar configurada em " + file, nil
}

// editStatusLine removes the status line only when it is still exactly the one Sidecar added.
func editStatusLine(file, command string, remove bool) error {
	if !remove || !fileExists(file) {
		return nil
	}
	obj, err := readObject(file)
	if err != nil {
		return err
	}
	cur, _ := obj["statusLine"].(map[string]any)
	if cur == nil || cur["command"] != command {
		return nil // the user changed it since: leave it alone
	}
	delete(obj, "statusLine")
	return writeObject(file, obj)
}

func ownStatusLine(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	cmd, _ := m["command"].(string)
	return IsOursStatusLine(cmd)
}

func settingsFile(env Env, agent string) string {
	switch agent {
	case "codex":
		return env.CodexHooks
	case "antigravity":
		return env.AntigravityHooks
	}
	return env.ClaudeSettings
}

// editAgentHooks dispatches to the editor of the agent's hook file for one event.
func editAgentHooks(agent, file, event, command string, add bool) error {
	if agent == "antigravity" {
		return editAntigravityHooks(file, event, command, add)
	}
	return editHooks(file, event, command, add)
}

// antigravityGroup is the hook group Sidecar owns in Antigravity CLI's hooks.json. That
// file maps group names to events, so Sidecar's handler lives under its own group.
const antigravityGroup = "sidecar"

// editAntigravityHooks adds or removes Sidecar's handler for one event of the sidecar group. The
// PreInvocation handler runs once per model call and receives the prompt. Other groups and
// handlers are left as they are.
func editAntigravityHooks(file, event, command string, add bool) error {
	obj, err := readObject(file)
	if err != nil {
		return err
	}
	group, _ := obj[antigravityGroup].(map[string]any)
	if obj[antigravityGroup] != nil && group == nil {
		return fmt.Errorf("group %q is not an object; left untouched", antigravityGroup)
	}
	if group == nil {
		group = map[string]any{}
	}
	var kept []any
	for _, h := range listOf(group[event]) {
		m, _ := h.(map[string]any)
		if cmd, _ := m["command"].(string); cmd == command {
			continue
		}
		kept = append(kept, h)
	}
	if add {
		kept = append(kept, map[string]any{"type": "command", "command": command, "timeout": hookTimeoutSeconds})
	}
	if len(kept) > 0 {
		group[event] = kept
	} else {
		delete(group, event)
	}
	if len(group) > 0 {
		obj[antigravityGroup] = group
	} else {
		delete(obj, antigravityGroup)
	}
	if !add && len(obj) == 0 && !fileExists(file) {
		return nil
	}
	return writeObject(file, obj)
}

// editHooks adds (add=true) or removes Sidecar's handler for one event: UserPromptSubmit for the
// prompt hook, or a tool event under --observe-tools. Other groups, handlers and events are kept
// in place. Unparseable files are never rewritten.
func editHooks(file, event, command string, add bool) error {
	obj, err := readObject(file)
	if err != nil {
		return err
	}
	hooks, _ := obj["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	var kept []any
	insertAt := -1
	for _, g := range listOf(hooks[event]) {
		group, ok := g.(map[string]any)
		if !ok {
			kept = append(kept, g)
			continue
		}
		var keep []any
		ours := false
		for _, h := range listOf(group["hooks"]) {
			cmd, _ := h.(map[string]any)["command"].(string)
			// Only change the exact command managed by this install. Another
			// sidecar executable on PATH may be an unrelated user hook.
			if cmd == command {
				ours = true
				if insertAt < 0 {
					insertAt = len(kept)
				}
				continue
			}
			keep = append(keep, h)
		}
		switch {
		case !ours:
			kept = append(kept, group)
		case len(keep) > 0:
			group["hooks"] = keep
			kept = append(kept, group)
		}
	}
	if add {
		handler := map[string]any{"type": "command", "command": command, "timeout": hookTimeoutSeconds}
		if insertAt < 0 {
			kept = append(kept, newGroup(event, handler))
		} else {
			group := newGroup(event, handler)
			kept = append(kept[:insertAt], append([]any{group}, kept[insertAt:]...)...)
		}
	}
	if len(kept) == 0 {
		delete(hooks, event)
	} else {
		hooks[event] = kept
	}
	if len(hooks) == 0 {
		delete(obj, "hooks")
	} else {
		obj["hooks"] = hooks
	}
	if !add && len(obj) == 0 && !fileExists(file) {
		return nil
	}
	return writeObject(file, obj)
}

// newGroup wraps a handler in a matcher group. Tool events use the "*" matcher: Sidecar observes
// every tool, and the matcher is what Claude Code reads to decide which calls the handler sees.
func newGroup(event string, handler map[string]any) map[string]any {
	g := map[string]any{"hooks": []any{handler}}
	if event == "PreToolUse" || event == "PostToolUse" {
		g["matcher"] = "*"
	}
	return g
}

func listOf(v any) []any {
	l, _ := v.([]any)
	return l
}

func readObject(file string) (map[string]any, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("not valid JSON, left untouched: %w", err)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	return obj, nil
}

func writeObject(file string, obj map[string]any) error {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(file, append(data, '\n'), 0o600)
}

func isEmptySettings(file string) bool {
	obj, err := readObject(file)
	return err == nil && len(obj) == 0
}

func backupOnce(file string) error {
	backup := file + BackupSuffix
	if fileExists(backup) {
		return nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return writeFileAtomic(backup, data, 0o600)
}

func copyExecutable(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeFileAtomic(dst, data, 0o755)
}

func writeFileAtomic(file string, data []byte, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	tmp := file + ".sidecar-tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func loadManifest(path string) manifest {
	m, err := readManifest(path)
	if err != nil {
		return manifest{}
	}
	return m
}

func readManifest(path string) (manifest, error) {
	var m manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

func saveManifest(path string, m manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'), 0o600)
}

func (m *manifest) upsert(e entry) {
	for i := range m.Entries {
		if m.Entries[i].Agent == e.Agent && m.Entries[i].hookEvent() == e.hookEvent() {
			// Keep the original FileCreated: only the first install knows whether the file was ours.
			e.FileCreated = m.Entries[i].FileCreated
			m.Entries[i] = e
			return
		}
	}
	m.Entries = append(m.Entries, e)
}

func (m manifest) createdFor(file string) bool {
	for _, e := range m.Entries {
		if e.File == file {
			return e.FileCreated
		}
	}
	return false
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func onPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func samePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}
