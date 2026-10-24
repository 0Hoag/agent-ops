#!/usr/bin/env bash
# Starts OpenCode for this repo (coordinator). Skips ~/.claude and ~/.agents skills so the
# GitHub/Codex orchestrator-workers skill from opencode.json is the only one with that name.
OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1 exec opencode "$@"
