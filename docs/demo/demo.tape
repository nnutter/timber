# Reproducible README demo: full worktree lifecycle in isolation.
# Render with: mise run demo  (requires Go toolchain + ffmpeg)
Output docs/demo/demo.gif
Output docs/demo/demo.mp4

Set Shell bash
Set FontSize 16
Set Width 1100
Set Height 700
Set Padding 16

Hide
Type "export DEMO_HOME=$(mktemp -d)"
Enter
Type "export HOME=$DEMO_HOME XDG_DATA_HOME=$DEMO_HOME/.local/share TIMBER_WORKTREE_ROOT=$DEMO_HOME/worktrees GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null HERDR_ENV="
Enter
Type "go build -o $DEMO_HOME/bin/timber ."
Enter
Type "export PATH=$DEMO_HOME/bin:$PATH"
Enter
Type "git init -q --bare -b main $DEMO_HOME/remote.git"
Enter
Type "git clone -q $DEMO_HOME/remote.git $DEMO_HOME/seed"
Enter
Type "git -C $DEMO_HOME/seed -c user.name=demo -c user.email=demo@example.com commit -q --allow-empty -m init"
Enter
Type "git -C $DEMO_HOME/seed branch -M main"
Enter
Type "git -C $DEMO_HOME/seed push -q -u origin main"
Enter
Type "git --git-dir=$DEMO_HOME/remote.git symbolic-ref HEAD refs/heads/main"
Enter
Type "clear"
Enter
Show

Type "timber repo add $DEMO_HOME/remote.git --name demo"
Enter
Sleep 1s

Type "timber create feature/login@demo"
Enter
Sleep 1s

Type "timber list @demo"
Enter
Sleep 2s
Screenshot docs/demo/demo-list.png

Type "cd $DEMO_HOME/worktrees/demo/feature/login/demo"
Enter
Sleep 1s

Type "ls -a"
Enter
Sleep 1s

Type "git status -sb"
Enter
Sleep 1s

Type "timber remove"
Enter
Sleep 2s
