package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type config struct {
	Addr         string
	DatabasePath string
	SelfCheck    bool
}

func parseConfig(args []string) (config, error) {
	set := flag.NewFlagSet("bioacoustic-release-hub", flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	var cfg config
	set.StringVar(&cfg.Addr, "addr", "", "监听地址，必须为回环地址")
	set.StringVar(&cfg.DatabasePath, "db", "bioacoustic-release-hub.db", "SQLite 文件路径")
	set.BoolVar(&cfg.SelfCheck, "selfcheck", false, "执行完整 HTTP 回环自检后退出")
	if err := set.Parse(args); err != nil {
		return cfg, err
	}
	if set.NArg() != 0 {
		return cfg, fmt.Errorf("存在未识别参数: %s", strings.Join(set.Args(), " "))
	}
	if cfg.Addr == "" {
		if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
			number, err := strconv.Atoi(port)
			if err != nil || number < 1 || number > 65535 {
				return cfg, fmt.Errorf("PORT 必须是 1 到 65535 的端口号")
			}
			cfg.Addr = net.JoinHostPort("127.0.0.1", port)
		} else {
			cfg.Addr = "127.0.0.1:19081"
		}
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return cfg, fmt.Errorf("-addr 必须采用 host:port 格式: %w", err)
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return cfg, fmt.Errorf("监听地址必须是回环地址，拒绝 %q", host)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return cfg, fmt.Errorf("监听端口必须处于 1 到 65535")
	}
	if cfg.SelfCheck {
		cfg.DatabasePath = ":memory:"
	}
	return cfg, nil
}
