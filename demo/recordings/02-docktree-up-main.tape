Output demo/out/02-docktree-up-main.gif
Output demo/out/02-docktree-up-main.mp4

Set FontSize 16
Set Width 1200
Set Height 600
Set Padding 20
Set Theme "Dracula"
Set TypingSpeed 8ms

Require cd

Type@50ms "cd demo/worktrees/main"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;36mWorktree A: demo-main branch\033[0m'"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mStarting Docktree in this worktree…\033[0m'"
Enter
Sleep 500ms

Type@50ms "../../../docktree up --build"
Enter
Sleep 15s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;35mStatus:\033[0m'"
Enter
Sleep 300ms

Type@50ms "../../../docktree status"
Enter
Sleep 2s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;35mAllocated ports:\033[0m'"
Enter
Sleep 300ms

Type@50ms "../../../docktree ports"
Enter
Sleep 2s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;32m✓ Worktree A is running on its own isolated port.\033[0m'"
Enter
Sleep 2s
