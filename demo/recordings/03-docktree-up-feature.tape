Output demo/out/03-docktree-up-feature.gif
Output demo/out/03-docktree-up-feature.mp4

Set FontSize 16
Set Width 1200
Set Height 600
Set Padding 20
Set Theme "Dracula"
Set TypingSpeed 8ms

Require cd

Type@50ms "cd demo/worktrees/feature"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;36mWorktree B: demo-feature branch\033[0m'"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mSame compose file, same service, same port 3000…\033[0m'"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mBut Docktree isolates it automatically.\033[0m'"
Enter
Sleep 500ms

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

Type@50ms "echo -e '\033[1;35mAllocated ports (different from Worktree A):\033[0m'"
Enter
Sleep 300ms

Type@50ms "../../../docktree ports"
Enter
Sleep 2s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;32m✓ Worktree B is running on a DIFFERENT isolated port.\033[0m'"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;32mNo conflicts. Both worktrees alive simultaneously.\033[0m'"
Enter
Sleep 2s
