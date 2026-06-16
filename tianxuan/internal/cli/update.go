package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"tianxuan/internal/update"
)

func updateCommand(args []string, currentVersion string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	repo := fs.String("repo", "tianxuanX/tianxuan", "GitHub 仓库（owner/repo）")
	tag := fs.String("tag", "", "指定版本标签（默认最新）")
	check := fs.Bool("check", false, "仅检查更新，不下载")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *check {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		// 只做版本比对，不下载
		rel, err := update.FetchLatestRelease(ctx, *repo, "")
		if err != nil {
			fmt.Fprintln(os.Stderr, "✖ 检查更新失败:", err)
			return 1
		}
		fmt.Printf("当前版本: %s\n", currentVersion)
		fmt.Printf("最新版本: %s\n", rel.TagName)
		fmt.Printf("发布时间: %s\n", rel.PublishedAt)
		fmt.Printf("仓库: https://github.com/%s\n", *repo)
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Printf("正在检查更新（%s）...\n", *repo)
	if err := update.Self(ctx, *repo, currentVersion, *tag); err != nil {
		fmt.Fprintln(os.Stderr, "✖ 更新失败:", err)
		return 1
	}
	return 0
}
