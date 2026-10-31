// plost-import-umami moves history from an Umami PostgreSQL database into a stopped Plost instance.
// It is a separate program so the PostgreSQL driver never becomes part of the Plost server.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/plost-dev/plost/internal/plost"
)

const usage = `用法：
  plost-import-umami list
  plost-import-umami import   -data 数据目录 -site Umami站点ID=域名 [-site ...] [选项]
  plost-import-umami rollback -data 数据目录 -site 域名 [-apply]

Umami 数据库连接地址只从环境变量 UMAMI_DATABASE_URL 读取（例如 postgres://用户@主机/库?sslmode=require），
不接受命令行参数，避免出现在进程列表和 shell 历史里。程序只在只读事务中读取 Umami，不会修改它。

import 默认只做预演：完整读取并核对，但不写入任何东西。确认后加 -apply 才真正写入，此时必须先停止 Plost。
选项：
  -create-sites     目标域名在 Plost 里不存在时自动创建（默认私有）
  -timezone 时区    新建站点的报表时区，例如 Asia/Shanghai（默认 Etc/UTC）；已有站点沿用它自己的时区
  -since 时间       只导入此时间（含）之后的事件，格式 2026-01-01 或 2026-01-01T00:00:00Z（UTC）
  -until 时间       只导入此时间（不含）之前的事件；建议设为把统计脚本换成 Plost 的时刻
  -strict           遇到 Plost 无法表示的事件就失败，而不是跳过并报告
  -allow-overlap    允许导入范围与 Plost 已有的实时数据重叠（可能重复计数）
  -report 文件      把核对清单写成 JSON（-apply 时默认写到数据目录的 backups/ 下）
`

type siteFlags map[string]string

func (s siteFlags) String() string { return "" }
func (s siteFlags) Set(v string) error {
	id, target, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(target) == "" {
		return errors.New("格式应为 Umami站点ID=域名")
	}
	s[strings.TrimSpace(id)] = strings.TrimSpace(target)
	return nil
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		fmt.Fprint(os.Stderr, usage)
		if len(os.Args) < 2 {
			os.Exit(2)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch os.Args[1] {
	case "list":
		err = list(ctx, os.Args[2:])
	case "import":
		err = importCmd(ctx, os.Args[2:])
	case "rollback":
		err = rollbackCmd(ctx, os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误："+err.Error())
		os.Exit(1)
	}
}

func connect(ctx context.Context) (*umamiDB, error) {
	dsn := os.Getenv("UMAMI_DATABASE_URL")
	if dsn == "" {
		return nil, errors.New("请通过环境变量 UMAMI_DATABASE_URL 提供 Umami 的 PostgreSQL 连接地址")
	}
	return openUmami(ctx, dsn)
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func list(ctx context.Context, args []string) error {
	if err := newFlags("list").Parse(args); err != nil {
		return err
	}
	db, err := connect(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	sites, err := db.Websites(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Umami 里有 %d 个站点：\n", len(sites))
	for _, w := range sites {
		counts, err := db.EventTypeCounts(ctx, w.ID, plost.ImportWindow{})
		if err != nil {
			return err
		}
		var other int64
		for kind, n := range counts {
			if kind != 1 && kind != 2 {
				other += n
			}
		}
		fmt.Printf("  %s  %s  %s\n      浏览 %d · 自定义事件 %d · 其他类型 %d（不会导入）\n", w.ID, w.Name, w.Domain, counts[1], counts[2], other)
	}
	fmt.Println("\n用 -site 站点ID=Plost域名 选择要导入的站点；没有选择的站点不会被导入。")
	return nil
}

func parseTime(name, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("-%s 的格式应为 2026-01-01 或 2026-01-01T00:00:00Z", name)
}

func importCmd(ctx context.Context, args []string) error {
	fs, sites := newFlags("import"), siteFlags{}
	data := fs.String("data", "", "")
	fs.Var(sites, "site", "")
	create, strict, overlap, apply := fs.Bool("create-sites", false, ""), fs.Bool("strict", false, ""), fs.Bool("allow-overlap", false, ""), fs.Bool("apply", false, "")
	timezone, since, until, report := fs.String("timezone", "", ""), fs.String("since", "", ""), fs.String("until", "", ""), fs.String("report", "", "")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, usage)
	}
	if *data == "" || len(sites) == 0 {
		return fmt.Errorf("需要 -data 和至少一个 -site\n\n%s", usage)
	}
	var window plost.ImportWindow
	var err error
	if window.Since, err = parseTime("since", *since); err != nil {
		return err
	}
	if window.Until, err = parseTime("until", *until); err != nil {
		return err
	}
	imp, err := plost.OpenImporter(*data, *apply)
	if err != nil {
		return err
	}
	defer imp.Close()
	db, err := connect(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	rep, runErr := imp.Run(ctx, db, plost.ImportOptions{Sites: sites, CreateSites: *create, Timezone: *timezone, Window: window, Apply: *apply, Strict: *strict, AllowOverlap: *overlap})
	printReport(rep, runErr)
	path := *report
	if path == "" && *apply && runErr == nil {
		suffix := make([]byte, 3)
		_, _ = rand.Read(suffix)
		path = filepath.Join(*data, "backups", "import-umami-"+time.Now().UTC().Format("20060102T150405Z")+"-"+hex.EncodeToString(suffix)+".json")
	}
	if path != "" && len(rep.Sites) > 0 {
		if err := writeManifest(path, rep); err != nil {
			if *apply && runErr == nil {
				return fmt.Errorf("导入已完成并已提交，但无法写入核对清单：%w", err)
			}
			return fmt.Errorf("无法写入核对清单：%w", err)
		}
		fmt.Println("核对清单：", path)
	}
	return runErr
}

func writeManifest(path string, rep plost.ImportReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(rep); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func reasons(m map[string]int64) string {
	var parts []string
	for k, n := range m {
		parts = append(parts, fmt.Sprintf("%s×%d", reasonName(k), n))
	}
	sort.Strings(parts)
	return strings.Join(parts, "，")
}

func kinds(m map[string]int64) string {
	var parts []string
	for k, n := range m {
		parts = append(parts, fmt.Sprintf("类型 %s 共 %d 条", k, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, "，")
}

func reasonName(k string) string {
	switch k {
	case "empty_event_name":
		return "自定义事件没有名称"
	case "reserved_event_name":
		return "自定义事件名与 pageview 冲突"
	case "event_name_too_long":
		return "事件名超过 255 字节"
	}
	return strings.Replace(k, "event_type_", "事件类型 ", 1)
}

func printReport(rep plost.ImportReport, err error) {
	mode := "预演（没有写入任何数据）"
	if rep.Apply {
		mode = "已写入"
		if err != nil {
			mode = "写入已回滚"
		}
	}
	fmt.Printf("Umami → Plost · %s\n", mode)
	if rep.Since != "" || rep.Until != "" {
		fmt.Printf("时间范围：%s ~ %s（UTC，含起不含止）\n", orOpen(rep.Since), orOpen(rep.Until))
	}
	for _, s := range rep.Sites {
		create := ""
		if s.CreatedSite {
			create = "（新建站点）"
		}
		fmt.Printf("\n%s (%s) → %s%s  时区 %s\n", s.Name, s.WebsiteID, s.Domain, create, s.Timezone)
		fmt.Printf("  读取 %d 条：浏览 %d，自定义事件 %d", s.Read, s.Pageviews, s.Custom)
		if rep.Apply {
			fmt.Printf("；新写入 %d，之前已导入 %d", s.Inserted, s.AlreadyImported)
		}
		fmt.Println()
		if len(s.Skipped) > 0 {
			fmt.Printf("  跳过：%s\n", reasons(s.Skipped))
		}
		if len(s.OtherTypes) > 0 {
			fmt.Printf("  不导入的事件类型：%s\n", kinds(s.OtherTypes))
		}
		if s.WithoutVisitID > 0 {
			fmt.Printf("  %d 条事件没有访问编号，按每个访客一次访问计算\n", s.WithoutVisitID)
		}
		if s.DroppedProps > 0 {
			fmt.Printf("  %d 个事件属性超出 Plost 的限制而丢弃\n", s.DroppedProps)
		}
		if s.FirstEvent != "" {
			fmt.Printf("  事件时间：%s ~ %s\n", s.FirstEvent, s.LastEvent)
		}
		if len(s.Problems) == 0 {
			what := "读取完整，与 Umami 自身的统计一致（预演不写入，没有核对 Plost 一侧）"
			if rep.Apply {
				what = "Plost 写入后重新统计，与 Umami 自身的统计一致"
			}
			fmt.Printf("  已逐日核对 %d 天（浏览数、自定义事件数、访客数、访问数）：%s\n", len(s.Days), what)
		}
		for _, p := range s.Problems {
			fmt.Println("  ✗", p)
		}
	}
	if len(rep.Unmapped) > 0 {
		fmt.Println("\n没有选择、不会导入的 Umami 站点：")
		for _, u := range rep.Unmapped {
			fmt.Printf("  %s  %s  %s  浏览 %d，自定义事件 %d\n", u.WebsiteID, u.Name, u.Domain, u.EventTypes["1"], u.EventTypes["2"])
		}
	}
	if rep.Snapshot != "" {
		fmt.Println("\n导入前的快照：", rep.Snapshot)
	}
	if err == nil && !rep.Apply {
		fmt.Println("\n预演完成。确认无误后，先停止 Plost，再加上 -apply 重新运行。")
	}
	if err == nil && rep.Apply {
		fmt.Println("\n导入完成。启动 Plost 即可查看；需要撤销时运行 rollback。")
	}
}

func orOpen(s string) string {
	if s == "" {
		return "不限"
	}
	return s
}

func rollbackCmd(ctx context.Context, args []string) error {
	fs := newFlags("rollback")
	data, apply := fs.String("data", "", ""), fs.Bool("apply", false, "")
	domain := fs.String("site", "", "")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, usage)
	}
	if *data == "" || *domain == "" {
		return fmt.Errorf("需要 -data 和 -site 域名\n\n%s", usage)
	}
	imp, err := plost.OpenImporter(*data, *apply)
	if err != nil {
		return err
	}
	defer imp.Close()
	rep, err := imp.RollbackUmami(ctx, *domain, *apply)
	if err != nil {
		return err
	}
	if !*apply {
		fmt.Printf("%s 里有 %d 条从 Umami 导入的事件。预演没有删除任何数据；加上 -apply 才会删除，Plost 自己采集的事件不受影响。\n", rep.Domain, rep.Imported)
		return nil
	}
	fmt.Printf("已从 %s 删除 %d 条从 Umami 导入的事件，快照：%s\n", rep.Domain, rep.Removed, rep.Snapshot)
	return nil
}
