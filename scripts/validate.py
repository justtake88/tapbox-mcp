import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
MCP_URL = "https://beta.stat.tapbox.ru/api/mcp"
errors = []


def load(rel):
    p = ROOT / rel
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except FileNotFoundError:
        errors.append(f"{rel}: файла нет")
    except json.JSONDecodeError as e:
        errors.append(f"{rel}: нечитаемый JSON — {e}")
    return None


market = load(".claude-plugin/marketplace.json")
codex_market = load(".agents/plugins/marketplace.json")
plugin = load("plugins/tapbox-stat/.claude-plugin/plugin.json")
codex_plugin = load("plugins/tapbox-stat/.codex-plugin/plugin.json")
codex_mcp = load("plugins/tapbox-stat/codex.mcp.json")
gemini = load("gemini-extension.json")

if market and plugin:
    entry = next((p for p in market.get("plugins", []) if p.get("name") == plugin.get("name")), None)
    if not entry:
        errors.append("marketplace.json: нет записи для плагина " + str(plugin.get("name")))
    else:
        if entry.get("version") != plugin.get("version"):
            errors.append("версия в marketplace.json не совпадает с plugin.json")
        if not (ROOT / entry.get("source", "")).is_dir():
            errors.append("marketplace.json: source указывает на несуществующую папку")

if plugin and codex_plugin:
    if plugin.get("name") != codex_plugin.get("name"):
        errors.append("имена плагинов Claude Code и Codex различаются")
    if plugin.get("version") != codex_plugin.get("version"):
        errors.append("версии плагинов Claude Code и Codex различаются")

if codex_market:
    for p in codex_market.get("plugins", []):
        path = (p.get("source") or {}).get("path", "")
        if not (ROOT / path).is_dir():
            errors.append(f".agents/plugins/marketplace.json: нет папки {path}")

urls = []
if plugin:
    urls += [s.get("url") for s in (plugin.get("mcpServers") or {}).values()]
    headers = [h for s in (plugin.get("mcpServers") or {}).values() for h in (s.get("headers") or {}).values()]
    for key in re.findall(r"\$\{user_config\.([a-z_]+)\}", " ".join(headers)):
        if key not in (plugin.get("userConfig") or {}):
            errors.append(f"plugin.json: ${{user_config.{key}}} не объявлен в userConfig")
if codex_mcp:
    urls += [s.get("url") for s in codex_mcp.get("mcpServers", {}).values()]
if gemini:
    urls += [s.get("httpUrl") for s in gemini.get("mcpServers", {}).values()]
for u in urls:
    if u != MCP_URL:
        errors.append(f"адрес MCP-сервера {u!r} отличается от {MCP_URL}")

skill = ROOT / "plugins/tapbox-stat/skills/tapbox-stat/SKILL.md"
if not skill.is_file():
    errors.append("нет SKILL.md")
else:
    head = skill.read_text(encoding="utf-8").split("---")
    if len(head) < 3 or "name:" not in head[1] or "description:" not in head[1]:
        errors.append("SKILL.md: во фронтматтере нужны name и description")

for p in ROOT.rglob("*"):
    if p.is_file() and ".git" not in p.parts and p.suffix in {".json", ".md", ".py", ".toml", ".yml"}:
        if re.search(r"tbx_[0-9a-f]{40}", p.read_text(encoding="utf-8", errors="ignore")):
            errors.append(f"{p.relative_to(ROOT)}: похоже на настоящий API-ключ")

if errors:
    print("Ошибки:")
    for e in errors:
        print(" -", e)
    sys.exit(1)
print("Всё в порядке")
