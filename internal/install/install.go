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
	"strings"

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
}

// UninstallOptions controls Uninstall.
type UninstallOptions struct {
	Env Env
	Out io.Writer
}

type entry struct {
	Agent       string `json:"agent"`
	File        string `json:"file"`
	Command     string `json:"command"`
	FileCreated bool   `json:"file_created"`
}

type manifest struct {
	Binary        string  `json:"binary,omitempty"`
	BinaryCreated bool    `json:"binary_created,omitempty"`
	Entries       []entry `json:"entries"`
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
	plan := describe(opts.Env, agents)
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
		command := HookCommand(dst, agent)
		created := !fileExists(file)
		if !created {
			if err := backupOnce(file); err != nil {
				return done, err
			}
		}
		if err := editAgentHooks(agent, file, command, true); err != nil {
			return done, fmt.Errorf("%s: %w", file, err)
		}
		m.upsert(entry{Agent: agent, File: file, Command: command, FileCreated: created || m.createdFor(file)})
		done = append(done, fmt.Sprintf("%s: hook instalado em %s", agent, file))
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
	for _, e := range m.Entries {
		if err := editAgentHooks(e.Agent, e.File, e.Command, false); err != nil {
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
	q := bin
	if runtime.GOOS == "windows" || strings.ContainsAny(bin, " \t\"'&()") {
		q = `"` + strings.ReplaceAll(bin, `"`, `\"`) + `"`
	}
	return q + " hook --agent " + agent
}

// IsOurs reports whether a hook command was installed by Sidecar for an agent.
func IsOurs(command, agent string) bool {
	suffix := " hook --agent " + agent
	if !strings.HasSuffix(command, suffix) {
		return false
	}
	bin := strings.Trim(strings.TrimSuffix(command, suffix), `"`)
	name := bin[strings.LastIndexAny(bin, `/\`)+1:]
	return name == "sidecar" || name == "sidecar.exe"
}

func describe(env Env, agents []string) []string {
	lines := []string{fmt.Sprintf("Instalar o binário em %s", filepath.Join(env.InstallDir, BinaryName))}
	for _, a := range agents {
		event := "UserPromptSubmit"
		if a == "antigravity" {
			event = "PreInvocation"
		}
		line := fmt.Sprintf("%s: adicionar hook %s em %s", a, event, settingsFile(env, a))
		if a == "codex" {
			line += " (o Codex pedirá para revisar e aprovar o hook na primeira execução)"
		}
		lines = append(lines, line)
	}
	return lines
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

// editAgentHooks dispatches to the editor of the agent's hook file.
func editAgentHooks(agent, file, command string, add bool) error {
	if agent == "antigravity" {
		return editAntigravityHooks(file, command, add)
	}
	return editHooks(file, command, add)
}

// antigravityGroup is the hook group Sidecar owns in Antigravity CLI's hooks.json. That
// file maps group names to events, so Sidecar's handler lives under its own group.
const antigravityGroup = "sidecar"

// editAntigravityHooks adds or removes Sidecar's PreInvocation handler, which runs once per
// model call and receives the prompt. Other groups and handlers are left as they are.
func editAntigravityHooks(file, command string, add bool) error {
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
	for _, h := range listOf(group["PreInvocation"]) {
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
		group["PreInvocation"] = kept
	} else {
		delete(group, "PreInvocation")
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

// editHooks adds (add=true) or removes Sidecar's UserPromptSubmit handler. Other groups,
// handlers and events are kept in place. Unparseable files are never rewritten.
func editHooks(file, command string, add bool) error {
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
	for _, g := range listOf(hooks["UserPromptSubmit"]) {
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
			kept = append(kept, map[string]any{"hooks": []any{handler}})
		} else {
			group := map[string]any{"hooks": []any{handler}}
			kept = append(kept[:insertAt], append([]any{group}, kept[insertAt:]...)...)
		}
	}
	if len(kept) == 0 {
		delete(hooks, "UserPromptSubmit")
	} else {
		hooks["UserPromptSubmit"] = kept
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
		if m.Entries[i].Agent == e.Agent {
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
