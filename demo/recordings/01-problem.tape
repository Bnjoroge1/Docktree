Output demo/out/01-problem.gif
Output demo/out/01-problem.mp4

Set FontSize 16
Set Width 1200
Set Height 600
Set Padding 20
Set Theme "Dracula"
Set TypingSpeed 8ms

Require echo
Require cat

Type@50ms "echo -e '\033[1;36mThe Problem: Two worktrees, one compose file, one port.\033[0m'"
Enter
Sleep 500ms

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mBoth worktrees share the same docker-compose.yml:\033[0m'"
Enter
Sleep 300ms

Type@50ms "echo -e '  - Same service name: \033[1;32mweb\033[0m'"
Enter
Sleep 200ms

Type@50ms "echo -e '  - Same container_name: \033[1;32mdocktree-demo-web\033[0m'"
Enter
Sleep 200ms

Type@50ms "echo -e '  - Same host port: \033[1;32m3000\033[0m'"
Enter
Sleep 500ms
Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;31mWithout Docktree, starting the second worktree would fail:\033[0m'"
Enter
Sleep 200ms

Type@50ms "echo -e '  - \033[1;31mPort conflict\033[0m: port 3000 already in use'"
Enter
Sleep 200ms

Type@50ms "echo -e '  - \033[1;31mContainer name conflict\033[0m: docktree-demo-web already exists'"
Enter
Sleep 500ms
Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;33mHere is the shared compose file:\033[0m'"
Enter
Sleep 300ms

Type@50ms "cat demo/sample-app/docker-compose.yml"
Enter
Sleep 1s

Type@50ms "echo ''"
Enter
Sleep 300ms

Type@50ms "echo -e '\033[1;32mDocktree solves this by generating isolated compose overrides\033[0m'"
Enter
Sleep 200ms

Type@50ms "echo -e '\033[1;32mwith unique ports and container names per worktree.\033[0m'"
Enter
Sleep 2s
