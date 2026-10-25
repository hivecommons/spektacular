{{! The finished step's report of the knowledge entries update_changelog wrote during a standalone run. }}
### Report the knowledge written

If this run collected a `## Knowledge written` list in `{{working_context_path}}`, tell the user about every entry on it now, all in one message: which entry was written (its path), where it lives (its tier and store), and why it is worth keeping. The entries are already written; this is a report, not a question. A run in its own worktrees removes that notes file as it finishes, so use the list as you last wrote it. Once reported, remove the list from `{{working_context_path}}` if the file is still there. If there is no such list, say nothing about knowledge capture.
