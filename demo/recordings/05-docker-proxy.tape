Output demo/out/05-docker-proxy.gif
Output demo/out/05-docker-proxy.mp4

Set FontSize 16
Set Width 1200
Set Height 600
Set Padding 20
Set Theme "Dracula"
Set TypingSpeed 8ms

Require echo
Require docktree

Type@50ms "echo -e '\033[1;36m═══ Activating Docktree Built-in Proxy ═══\033[0m'"
Enter
Sleep 500ms

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mDocktree provides a built-in reverse proxy.\033[0m'"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mIt routes requests by hostname to the correct worktree port,\033[0m'"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mso you can access http://<instance>.localhost instead of ports.\033[0m'"
Enter
Sleep 500ms

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;35mStarting reverse proxy on port 8320…\033[0m'"
Enter
Sleep 500ms

# Start the proxy in the background, showing its routing table, then stop after a few seconds
Type@50ms "./docktree proxy &"
Enter
Sleep 2s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mSending requests to both worktrees using their domain names:\033[0m'"
Enter
Sleep 300ms
Type@50ms "curl -s -H 'Host: demo-repo-demo-main-fa7c81.localhost' http://127.0.0.1:8320/api"
Enter
Sleep 1s

Type@50ms "curl -s -H 'Host: demo-repo-demo-feature-db382c.localhost' http://127.0.0.1:8320/api"
Enter
Sleep 1s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;35mStopping reverse proxy…\033[0m'"
Enter
Sleep 500ms

Type@50ms "kill %1"
Enter
Sleep 1s

Type@50ms "echo -e '\033[1;32m✓ Proxy routing verified successfully.\033[0m'"
Enter
Sleep 2s
