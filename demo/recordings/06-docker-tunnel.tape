Output demo/out/06-docker-tunnel.gif
Output demo/out/06-docker-tunnel.mp4

Set FontSize 16
Set Width 1200
Set Height 600
Set Padding 20
Set Theme "Dracula"
Set TypingSpeed 8ms

Require echo
Require docktree
Require cd

Type@50ms "echo -e '\033[1;36m═══ Exposing Worktree via External Tunnel ═══\033[0m'"
Enter
Sleep 500ms

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mDocktree allows exposing worktrees externally.\033[0m'"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mThis starts a Cloudflare Tunnel (default) or ngrok,\033[0m'"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mrouting public traffic directly to the isolated worktree port.\033[0m'"
Enter
Sleep 500ms

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "cd demo/worktrees/main"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;35mStarting tunnel for main branch worktree…\033[0m'"
Enter
Sleep 500ms

Type@50ms "../../../docktree tunnel start"
Enter
Sleep 6s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;35mChecking tunnel status:\033[0m'"
Enter
Sleep 500ms

Type@50ms "../../../docktree tunnel status"
Enter
Sleep 2s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;35mStopping the tunnel:\033[0m'"
Enter
Sleep 500ms

Type@50ms "../../../docktree tunnel stop"
Enter
Sleep 1s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;32m✓ Tunnel lifecycle verified successfully.\033[0m'"
Enter
Sleep 2s
