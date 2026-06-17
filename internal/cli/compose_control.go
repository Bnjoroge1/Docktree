package cli

func runCp(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "cp", ctx.Args[1:], printCpHelp)
}

func runWait(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "wait", ctx.Args[1:], printWaitHelp)
}

func runWatch(ctx *Context) (any, int, error) {
	return runComposePassthrough(ctx, "watch", ctx.Args[1:], printWatchHelp)
}
