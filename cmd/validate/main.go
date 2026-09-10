package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const mcpURL = "https://beta.stat.tapbox.ru/api/mcp"

const pluginDir = "plugins/tapbox-stat"

type server struct {
	URL     string            `json:"url"`
	HTTPURL string            `json:"httpUrl"`
	Headers map[string]string `json:"headers"`
}

type marketplace struct {
	Plugins []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Source  string `json:"source"`
	} `json:"plugins"`
}

type codexMarketplace struct {
	Plugins []struct {
		Name   string `json:"name"`
		Source struct {
			Path string `json:"path"`
		} `json:"source"`
	} `json:"plugins"`
}

type claudePlugin struct {
	Name       string                     `json:"name"`
	Version    string                     `json:"version"`
	UserConfig map[string]json.RawMessage `json:"userConfig"`
	MCPServers map[string]server          `json:"mcpServers"`
}

type codexPlugin struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	MCPServers string `json:"mcpServers"`
}

type mcpFile struct {
	MCPServers map[string]server `json:"mcpServers"`
}

type geminiExtension struct {
	Version    string            `json:"version"`
	MCPServers map[string]server `json:"mcpServers"`
}

var errs []string

func fail(format string, a ...any) {
	errs = append(errs, fmt.Sprintf(format, a...))
}

func load(root, rel string, v any) bool {
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		fail("%s: файла нет", rel)
		return false
	}
	if err := json.Unmarshal(b, v); err != nil {
		fail("%s: нечитаемый JSON — %v", rel, err)
		return false
	}
	return true
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	var market marketplace
	var codexMarket codexMarketplace
	var plugin claudePlugin
	var codex codexPlugin
	var gemini geminiExtension
	okMarket := load(root, ".claude-plugin/marketplace.json", &market)
	okCodexMarket := load(root, ".agents/plugins/marketplace.json", &codexMarket)
	okPlugin := load(root, pluginDir+"/.claude-plugin/plugin.json", &plugin)
	okCodex := load(root, pluginDir+"/.codex-plugin/plugin.json", &codex)
	okGemini := load(root, "gemini-extension.json", &gemini)

	if okMarket && okPlugin {
		found := false
		for _, p := range market.Plugins {
			if p.Name != plugin.Name {
				continue
			}
			found = true
			if p.Version != plugin.Version {
				fail("версия в marketplace.json (%s) не совпадает с plugin.json (%s)", p.Version, plugin.Version)
			}
			if !isDir(filepath.Join(root, p.Source)) {
				fail("marketplace.json: source %q указывает на несуществующую папку", p.Source)
			}
		}
		if !found {
			fail("marketplace.json: нет записи для плагина %s", plugin.Name)
		}
	}

	if okPlugin && okCodex {
		if plugin.Name != codex.Name {
			fail("имена плагинов Claude Code (%s) и Codex (%s) различаются", plugin.Name, codex.Name)
		}
		if plugin.Version != codex.Version {
			fail("версии плагинов Claude Code (%s) и Codex (%s) различаются", plugin.Version, codex.Version)
		}
	}

	if okPlugin && okGemini && gemini.Version != plugin.Version {
		fail("версия в gemini-extension.json (%s) не совпадает с plugin.json (%s)", gemini.Version, plugin.Version)
	}

	if okCodexMarket {
		for _, p := range codexMarket.Plugins {
			if !isDir(filepath.Join(root, p.Source.Path)) {
				fail(".agents/plugins/marketplace.json: нет папки %s", p.Source.Path)
			}
		}
	}

	var urls []string
	userRef := regexp.MustCompile(`\$\{user_config\.([A-Za-z0-9_]+)\}`)
	if okPlugin {
		for name, s := range plugin.MCPServers {
			urls = append(urls, s.URL)
			for _, h := range s.Headers {
				for _, m := range userRef.FindAllStringSubmatch(h, -1) {
					if _, ok := plugin.UserConfig[m[1]]; !ok {
						fail("plugin.json: сервер %s ссылается на ${user_config.%s}, а в userConfig такого ключа нет", name, m[1])
					}
				}
			}
		}
	}
	if okCodex && codex.MCPServers != "" {
		var f mcpFile
		if load(root, filepath.Join(pluginDir, codex.MCPServers), &f) {
			for _, s := range f.MCPServers {
				urls = append(urls, s.URL)
			}
		}
	}
	if okGemini {
		for _, s := range gemini.MCPServers {
			urls = append(urls, s.HTTPURL)
		}
	}
	for _, u := range urls {
		if u != mcpURL {
			fail("адрес MCP-сервера %q отличается от %s", u, mcpURL)
		}
	}

	if b, err := os.ReadFile(filepath.Join(root, pluginDir, "skills/tapbox-stat/SKILL.md")); err != nil {
		fail("нет SKILL.md")
	} else {
		parts := strings.SplitN(string(b), "---", 3)
		if len(parts) < 3 || !strings.Contains(parts[1], "name:") || !strings.Contains(parts[1], "description:") {
			fail("SKILL.md: во фронтматтере нужны name и description")
		}
	}

	secret := regexp.MustCompile(`tbx_[0-9a-f]{40}`)
	exts := map[string]bool{".json": true, ".md": true, ".go": true, ".toml": true, ".yml": true, ".yaml": true}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !exts[filepath.Ext(p)] {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil && secret.Match(b) {
			rel, _ := filepath.Rel(root, p)
			fail("%s: похоже на настоящий API-ключ", rel)
		}
		return nil
	})

	if len(errs) > 0 {
		fmt.Println("Ошибки:")
		for _, e := range errs {
			fmt.Println(" -", e)
		}
		os.Exit(1)
	}
	fmt.Println("Всё в порядке")
}
