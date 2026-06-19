package cli

func runBuild(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "build", ctx.Args[1:], true, printBuildHelp)
}

func runPull(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "pull", ctx.Args[1:], true, printPullHelp)
}

func runPush(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "push", ctx.Args[1:], true, printPushHelp)
}
