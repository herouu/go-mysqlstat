package main

import (
	"database/sql"
	"fmt"
	list "github.com/duke-git/lancet/v2/datastructure/list"
	"github.com/duke-git/lancet/v2/datetime"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/urfave/cli/v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

// DB 数据库链接单例
var DB *gorm.DB

var clear map[string]func() //create a map for storing clear funcs

// Quota 指标
type Quota struct {
	selectCount int
	insertCount int
	updateCount int
	deleteCount int
	maxConn     int
	conn        int
	recv        int
	send        int
}

type CalQuota struct {
	selectPerSecond int
	insertPerSecond int
	updatePerSecond int
	deletePerSecond int
	recvPerSecond   int
	sendPerSecond   int
	recvMbps        string
	sendMbps        string
	currentTime     string
	maxConn         int
	connCount       int
}

func main() {

	app := cli.NewApp()
	app.Name = "go-mysqlstat"
	app.Usage = "MySQL命令行监控工具 - mysqlstat"
	app.Commands = []*cli.Command{
		//{
		//	Name:   "install",
		//	Action: ctrlAction,
		//},
	}
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:     "mysql_ip",
			Aliases:  []string{"H"},
			Value:    "",
			Usage:    "Mysql IP",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "mysql_port",
			Aliases:  []string{"P"},
			Value:    "",
			Usage:    "Mysql Port",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "mysql_user",
			Aliases:  []string{"u"},
			Value:    "",
			Usage:    "Mysql User",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "mysql_password",
			Aliases:  []string{"p"},
			Value:    "",
			Usage:    "Mysql Password",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "top",
			Value:    "",
			Usage:    "需要提供一个整数类型的参数值，该参数值表示执行次数最频繁的前N条SQL语句",
			Required: false,
			Action:   topFrequentlySql,
		},
	}

	app.Action = mysqlStatusMonitor
	app.Version = "1.0.0"
	err := app.Run(os.Args)
	if err != nil {
		return
	}
}

func mysqlStatusMonitor(c *cli.Context) error {
	// 初始化数据库连接
	dbInit(c)
	sigs := make(chan os.Signal, 1)
	//注册信号处理函数
	// Ctrl+C Ctrl+Z
	signal.Notify(sigs, syscall.SIGINT)

	go func() {
		prev := calQuota()
		tw := table.NewWriter()
		tw.SetTitle("Real-time Monitoring")
		tw.SetStyle(table.Style{
			Name:    "StyleDefault",
			Box:     table.StyleBoxDefault,
			Color:   table.ColorOptionsDefault,
			Format:  table.FormatOptionsDefault,
			HTML:    table.DefaultHTMLOptions,
			Options: table.OptionsDefault,
			Title:   table.TitleOptions{Align: text.AlignCenter},
		})
		tw.AppendHeader(table.Row{"Time", "Select", "Insert", "Update", "Delete", "Conn", "Max_conn", "Recv", "Send"})

		time.Sleep(1 * time.Second)
		fmt.Println(tw.Render())
		li := list.NewList([]table.Row{})

		count := 1
		for {
			if count > 25 {
				tw.ResetRows()
				li.PopFirst()
				tw.AppendRows(li.Data())
			}
			current := calQuota()
			c := buildCalQuota(prev, current)
			prev = current
			row := table.Row{c.currentTime, c.selectPerSecond, c.insertPerSecond, c.updatePerSecond, c.deletePerSecond, c.connCount,
				c.maxConn, c.recvMbps, c.sendMbps}
			li.Push(row)
			tw.AppendRow(row)
			callClear()
			fmt.Println(tw.Render())
			time.Sleep(1 * time.Second)

			count++
			select {
			case <-sigs:
				os.Exit(0)
			default:
			}
		}
	}()
	<-sigs
	return nil
}

func dbInit(c *cli.Context) {
	ip := c.String("mysql_ip")
	pwd := c.String("mysql_password")
	port := c.String("mysql_port")
	name := c.String("mysql_user")
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/information_schema?charset=utf8mb4&parseTime=True&loc=Local", name, pwd, ip, port)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	DB = db
}

type DbResult struct {
	VariableName string `gorm:"column:Variable_name"`
	Value        int
}

// 指标计算
func calQuota() *Quota {

	var sc DbResult
	var ic DbResult
	var uc DbResult
	var dc DbResult
	var mc DbResult
	var br DbResult
	var bs DbResult
	var tc DbResult

	//获取数据库的初始统计信息
	DB.Raw("SHOW GLOBAL STATUS LIKE 'Com_select'").Scan(&sc)
	DB.Raw("SHOW GLOBAL STATUS LIKE 'Com_insert'").Scan(&ic)
	DB.Raw("SHOW GLOBAL STATUS LIKE 'Com_update'").Scan(&uc)
	DB.Raw("SHOW GLOBAL STATUS LIKE 'Com_delete'").Scan(&dc)
	DB.Raw("SHOW GLOBAL VARIABLES LIKE 'max_connections'").Scan(&mc)
	DB.Raw("SHOW GLOBAL STATUS LIKE 'Bytes_received'").Scan(&br)
	DB.Raw("SHOW GLOBAL STATUS LIKE 'Bytes_sent'").Scan(&bs)
	DB.Raw("SHOW GLOBAL STATUS LIKE 'Threads_connected'").Scan(&tc)
	return &Quota{selectCount: sc.Value, insertCount: ic.Value, updateCount: uc.Value, deleteCount: dc.Value,
		conn: tc.Value, recv: br.Value, maxConn: mc.Value, send: bs.Value}
}

func buildCalQuota(prev *Quota, c *Quota) *CalQuota {
	//计算每秒操作量和网络数据量
	selectPerSecond := c.selectCount - prev.selectCount
	insertPerSecond := c.insertCount - prev.insertCount
	updatePerSecond := c.updateCount - prev.updateCount
	deletePerSecond := c.deleteCount - prev.deleteCount
	recvPerSecond := c.recv - prev.recv
	sendPerSecond := c.send - prev.send

	//将每秒接收和发送数据量从字节转换为兆比特
	recvMbps := float64(recvPerSecond*8) / 1000000
	sendMbps := float64(sendPerSecond*8) / 1000000

	currentTime := datetime.FormatTimeToStr(time.Now(), "yyyy-MM-dd HH:mm:ss")
	return &CalQuota{selectPerSecond: selectPerSecond,
		insertPerSecond: insertPerSecond,
		updatePerSecond: updatePerSecond,
		deletePerSecond: deletePerSecond,
		recvPerSecond:   recvPerSecond,
		sendPerSecond:   sendPerSecond,
		currentTime:     currentTime,
		connCount:       c.conn,
		maxConn:         c.maxConn,
		recvMbps:        fmt.Sprintf("%.3f MBit/s", recvMbps),
		sendMbps:        fmt.Sprintf("%.3f MBit/s", sendMbps),
	}
}

func init() {
	clear = make(map[string]func()) //Initialize it
	clear["linux"] = func() {
		cmd := exec.Command("clear") //Linux example, its tested
		cmd.Stdout = os.Stdout
		err := cmd.Run()
		if err != nil {
			return
		}
	}
	clear["windows"] = func() {
		cmd := exec.Command("cmd", "/c", "cls") //Windows example, its tested
		cmd.Stdout = os.Stdout
		err := cmd.Run()
		if err != nil {
			return
		}
	}
}

func callClear() {
	value, ok := clear[runtime.GOOS] //runtime.GOOS -> linux, windows, darwin etc.
	if ok {                          //if we defined a clear func for that platform:
		value() //we execute it
	} else { //unsupported platform
		panic("Your platform is unsupported! I can't clear terminal screen :(")
	}
}

func topFrequentlySql(c *cli.Context, top string) error {
	// 初始化数据库连接
	dbInit(c)
	var isPerformanceSchema int
	DB.Raw("SELECT @@performance_schema").Scan(&isPerformanceSchema)

	if isPerformanceSchema == 0 {
		fmt.Println("performance_schema参数未开启。")
		fmt.Println("在my.cnf配置文件里添加performance_schema=1，并重启mysqld进程生效。")
		os.Exit(0)
	}

	DB.Raw("SET @sys.statement_truncate_len = 1024")
	rows, err := DB.Raw("select query,db,last_seen,exec_count,max_latency,avg_latency from sys.statement_analysis order by exec_count desc, "+
		"last_seen desc limit @top", sql.Named("top", top)).Rows()
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(rows)
	if err != nil {
		return err
	}
	// 表格
	tw := table.NewWriter()
	tw.SetTitle("Query Analysis")
	tw.SetStyle(table.Style{
		Name:    "StyleDefault",
		Box:     table.StyleBoxDefault,
		Color:   table.ColorOptionsDefault,
		Format:  table.FormatOptionsDefault,
		HTML:    table.DefaultHTMLOptions,
		Options: table.OptionsDefault,
		Title:   table.TitleOptions{Align: text.AlignCenter},
	})
	tw.AppendHeader(table.Row{"执行语句", "数据库名", "最近执行时间", "SQL执行总次数", "最大执行时间", "平均执行时间"})

	for rows.Next() {
		var query, db, execCount, maxLatency, avgLatency sql.NullString
		var lastSeen sql.NullTime
		err := rows.Scan(&query, &db, &lastSeen, &execCount, &maxLatency, &avgLatency)
		if err != nil {
			fmt.Println(err)
		}
		tw.AppendRow(table.Row{query.String, db.String, lastSeen.Time.Format("2006-01-02 15:04:05.000000"), execCount.String, maxLatency.String, avgLatency.String})
	}
	fmt.Println(tw.Render())
	return nil
}
