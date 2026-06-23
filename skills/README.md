# Docktree agent skills

Agent-installable skills for working with [Docktree](../README.md). Distributed via `npx skills` 

Works with Claude Code, Codex, Cursor, OpenCode, OMP/Pi and 60+ other agents.

## Install

```bash
# install all docktree skills into the current project (./.claude/skills/, etc.)
npx skills add Bnjoroge1/Docktree

# install globally, for one specific agent
npx skills add Bnjoroge1/Docktree -g -a claude-code

# list what's in here without installing
npx skills add Bnjoroge1/Docktree --list

# install one skill
npx skills add Bnjoroge1/Docktree --skill docktree
```

## Skills in this repo


| Skill                             | Purpose                                                                                                            |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `[docktree](./docktree/SKILL.md)` | How an agent should invoke the `docktree` CLI: which commands honor `--json`, lifecycle gotchas, common workflows. |




