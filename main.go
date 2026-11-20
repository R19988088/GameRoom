package main

import (
	"flag"
	"log"

	"github.com/R19988088/GameRoom/internal/app"
)

func main() {
	fetch := flag.Bool("fetch-covers", false, "补全缺少的街机封面后退出")
	info := flag.Bool("fetch-info", false, "从 Arcade Italia 补全厂商与年份后退出")
	dos := flag.String("import-dos", "", "从该目录的 list.csv 重建 DOS&PC98 货架后退出")
	play := flag.String("play", "", "启动后直接跑这款游戏（rom 文件名或标题的一部分），用于验收窗口内嵌")
	limit := flag.Int("limit", 0, "上面两个任务各自最多处理几个，0 为全部")
	flag.Parse()

	if *play != "" {
		app.AutoPlay(*play)
	}

	switch {
	case *fetch:
		if err := app.FetchCovers(*limit); err != nil {
			log.Fatal(err)
		}
		return
	case *info:
		if err := app.FetchInfo(*limit); err != nil {
			log.Fatal(err)
		}
		return
	case *dos != "":
		if err := app.ImportDOS(*dos); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
