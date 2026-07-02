Output demo/out/docktree-demo.gif
Output demo/out/docktree-demo.mp4

Set FontSize 16
Set Width 1200
Set Height 600
Set Padding 20
Set Theme "Dracula"
Set TypingSpeed 12ms

Require echo
Require docktree
Require cd
Require curl

# 1. Introduction and problem
Type@50ms "echo -e '\033[1;36m━━━ Docktree: Multi-worktree Docker Compose Coordination ━━━\033[0m'"
Enter
Sleep 300ms
Type@50ms "echo -e '\033[1;33mProblem: Two worktrees of the same app fight over ports & container names.\033[0m'"
Enter
Sleep 300ms
Type@50ms "echo -e '\033[1;33mSolution: Docktree isolates them automatically.\033[0m'"
Enter
Sleep 800ms

Type@50ms "echo ''"
Enter

# 2. Start Worktree A
Type@50ms "cd demo/worktrees/main"
Enter
Sleep 200ms
Type@50ms "echo -e '\033[1;36m[Worktree A] demo-main branch\033[0m'"
Enter
Sleep 200ms
Type@50ms "../../../docktree up --build"
Enter
Sleep 6s

Type@50ms "echo ''"
Enter

# 3. Start Worktree B
Type@50ms "cd ../feature"
Enter
Sleep 200ms
Type@50ms "echo -e '\033[1;36m[Worktree B] demo-feature branch\033[0m'"
Enter
Sleep 200ms
Type@50ms "../../../docktree up --build"
Enter
Sleep 6s

Type@50ms "echo ''"
Enter

# 4. View instances and ports
Type@50ms "cd ../../.."
Enter
Sleep 200ms
Type@50ms "echo -e '\033[1;35mChecking running instances:\033[0m'"
Enter
Sleep 200ms
Type@50ms "./docktree status --all"
Enter
Sleep 1s

Type@50ms "echo ''"
Enter
Type@50ms "echo -e '\033[1;35mChecking allocated ports:\033[0m'"
Enter
Sleep 200ms
Type@50ms "./docktree ports --all"
Enter
Sleep 1s

Type@50ms "echo ''"
Enter

# 5. Reverse Proxy
Type@50ms "echo -e '\033[1;36m━━━ Feature: Built-in Reverse Proxy ━━━\033[0m'"
Enter
Sleep 300ms
Type@50ms "echo -e '\033[1;33mStart a single local proxy to route by hostname.\033[0m'"
Enter
Sleep 300ms
Type@50ms "./docktree proxy &"
Enter
Sleep 2s

Type@50ms "echo ''"
Enter
Type@50ms "echo -e '\033[1;35mSending requests using Host headers:\033[0m'"
Enter
Sleep 200ms
Type@50ms "curl -s -H 'Host: demo-repo-demo-main-fa7c81.localhost' http://127.0.0.1:8320/api"
Enter
Sleep 800ms
Type@50ms "curl -s -H 'Host: demo-repo-demo-feature-db382c.localhost' http://127.0.0.1:8320/api"
Enter
Sleep 800ms

Type@50ms "echo ''"
Enter
Type@50ms "kill %1"
Enter
Sleep 800ms

# 6. External Tunnels
Type@50ms "echo -e '\033[1;36m━━━ Feature: External Sharing Tunnels ━━━\033[0m'"
Enter
Sleep 300ms
Type@50ms "echo -e '\033[1;33mExpose any worktree to the public internet using cloudflared.\033[0m'"
Enter
Sleep 300ms
Type@50ms "cd demo/worktrees/main"
Enter
Sleep 200ms
Type@50ms "../../../docktree tunnel start"
Enter
Sleep 4s

Type@50ms "echo ''"
Enter
Type@50ms "../../../docktree tunnel status"
Enter
Sleep 1s
Type@50ms "../../../docktree tunnel stop"
Enter
Sleep 800ms

Type@50ms "echo ''"
Enter

# 7. Clean Teardown (down)
Type@50ms "echo -e '\033[1;36m━━━ Act 4: Teardown (down) ━━━\033[0m'"
Enter
Sleep 300ms
Type@50ms "echo -e '\033[1;33mStop all active worktree compose deployments.\033[0m'"
Enter
Sleep 300ms
Type@50ms "../../../docktree down --all"
Enter
Sleep 4s

Type@50ms "echo ''"
Enter
Type@50ms "echo -e '\033[1;32m✓ All tasks completed successfully. Docktree is awesome!\033[0m'"
Enter
Sleep 1.5s
