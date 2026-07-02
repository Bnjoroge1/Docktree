Output demo/out/04-two-worktrees.gif
Output demo/out/04-two-worktrees.mp4

Set FontSize 16
Set Width 1200
Set Height 600
Set Padding 20
Set Theme "Dracula"
Set TypingSpeed 8ms

Require echo

Type@50ms "echo -e '\033[1;36m═══ Both Worktrees Running Simultaneously ═══\033[0m'"
Enter
Sleep 500ms

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;35mAll Docktree instances:\033[0m'"
Enter
Sleep 300ms

Type@50ms "./docktree status --all"
Enter
Sleep 2s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;35mAll allocated ports:\033[0m'"
Enter
Sleep 300ms

Type@50ms "./docktree ports --all"
Enter
Sleep 2s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mLive proof — curling both apps:\033[0m'"
Enter
Sleep 500ms

Type@50ms "./demo/scripts/show-both.sh"
Enter
Sleep 5s

Type@50ms "echo ''"
Enter
Sleep 500ms

Type@50ms "echo -e '\033[1;32m✓ Two versions of the same app, running at the same time.\033[0m'"
Enter
Sleep 300ms

Type@50ms "echo -e '  Same source compose service + port, isolated by Docktree.'"
Enter
Sleep 3s
