package main

import (
	"database/sql"
	"fmt"
	"github.com/duke-git/lancet/v2/convertor"
	list "github.com/duke-git/lancet/v2/datastructure/list"
	"github.com/duke-git/lancet/v2/datetime"
	"github.com/gookit/goutil/strutil"
	"github.com/gookit/goutil/timex"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/urfave/cli/v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
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
			Usage:    "需要提供一个整数类型的参数值，该参数值表示执行次数最频繁的前N条SQL语句",
			Required: false,
		},
		&cli.StringFlag{
			Name:     "io",
			Usage:    "需要提供一个整数类型的参数值，该参数值表示访问次数最频繁的前N张表文件ibd",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "lock",
			Usage:    "查看当前锁阻塞的SQL",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "kill",
			Usage:    "杀死当前锁阻塞的SQL",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "index",
			Usage:    "查看重复或冗余的索引",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "conn",
			Usage:    "查看应用端IP连接数总和",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "tinfo",
			Usage:    "统计库里每个表的大小",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "fpk",
			Usage:    "快速找出没有主键的表",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "dead",
			Usage:    "查看死锁信息",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "binlog",
			Usage:    "Binlog分析-高峰期排查哪些表TPS比较高",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "repl",
			Usage:    "查看主从复制信息",
			Required: false,
		},
	}

	app.Action = ctrlAction
	app.Version = "1.0.0"
	err := app.Run(os.Args)
	if err != nil {
		return
	}
}

func ctrlAction(context *cli.Context) error {

	ip := context.String("mysql_ip")
	pwd := context.String("mysql_password")
	port := context.String("mysql_port")
	name := context.String("mysql_user")

	// 初始化数据库
	dbInit(ip, pwd, port, name)

	topV := context.String("top")
	ioV := context.String("io")
	lock := context.Bool("lock")
	kill := context.Bool("kill")
	index := context.Bool("index")
	conn := context.Bool("conn")
	tinfo := context.Bool("tinfo")
	fpk := context.Bool("fpk")
	dead := context.Bool("dead")

	if strutil.IsNotBlank(topV) {
		showFrequentlySql(topV)
	} else if strutil.IsNotBlank(ioV) {
		showFrequentlyIo(ioV)
	} else if lock && kill {
		showLockSql(true)
	} else if lock {
		showLockSql(false)
	} else if kill {
		fmt.Println("Error: --kill requires --lock")
	} else if index {
		showRedundantIndexes()
	} else if conn {
		showConnCount(ip, port)
	} else if tinfo {
		showTableInfo()
	} else if fpk {
		showFpkInfo()
	} else if dead {
		showDeadlockInfo()
	} else {
		mysqlStatusMonitor()
	}
	return nil

}

func mysqlStatusMonitor() {
	// 初始化数据库连接

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
}

// dbInit 连接到 MySQL 数据库并初始化全局 DB 变量。
//
// 参数:
//
//	ip - 数据库的IP地址。
//	pwd - 数据库的密码。
//	port - 数据库的端口号。
//	name - 数据库的名称。
//
// 无返回值。
func dbInit(ip, pwd, port, name string) {

	// 构造数据库的连接字符串
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/information_schema?charset=utf8mb4&parseTime=True&loc=Local", name, pwd, ip, port)

	// 尝试使用提供的 DSN 连接到 MySQL 数据库
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err) // 如果连接失败，则中断程序
	}

	// 获取底层的 SQL DB 实例，以便进行更深层次的数据库连接配置
	sqlDB, err := db.DB()
	if err != nil {
		panic(err) // 如果获取失败，则中断程序
	}

	// 配置数据库连接池的最大打开连接数和最大空闲连接数
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// 全局变量 DB 被赋值为初始化好的数据库连接实例
	DB = db
}

type DbResult struct {
	VariableName string `gorm:"column:Variable_name"`
	Value        int
}

// 指标计算
func calQuota() *Quota {

	var sc, ic, uc, dc, mc, br, bs, tc DbResult
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
	} else {
		panic("Your platform is unsupported! I can't clear terminal screen :(")
	}
}

// showFrequentlySql 显示MySQL中最常执行的SQL语句分析。
//
// 参数:
//
//	top - 指定要显示的最常执行SQL的数量。
//
// 此函数会检查 performance_schema 是否启用，如果未启用则给出提示并返回。
// 否则，它将从 performance_schema 获取最常执行的SQL语句及其相关信息，
// 并以表格形式打印出来，包括执行语句、数据库名、最近执行时间、执行总次数、最大执行时间和平均执行时间。
func showFrequentlySql(top string) {

	// 检查 performance_schema 是否已启用
	var isPerformanceSchema int
	DB.Raw("SELECT @@performance_schema").Scan(&isPerformanceSchema)

	if isPerformanceSchema == 0 {
		fmt.Println("performance_schema参数未开启。")
		fmt.Println("在my.cnf配置文件里添加performance_schema=1，并重启mysqld进程生效。")
		return
	}

	// 设置 SQL 语句截断长度
	DB.Exec("SET @sys.statement_truncate_len = 1024")

	// 查询最常执行的SQL信息
	rows, err := DB.Raw("select query,db,last_seen,exec_count,max_latency,avg_latency from sys.statement_analysis order by exec_count desc,last_seen desc limit @top", sql.Named("top", top)).Rows()
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(rows)
	if err != nil {
		fmt.Println(err)
		return
	}

	// 创建表格并填充数据
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
		dateFormat := timex.New(lastSeen.Time).DateFormat(timex.TemplateWithMs3)
		tw.AppendRow(table.Row{query.String, db.String, dateFormat, execCount.String, maxLatency.String, avgLatency.String})
	}

	// 打印表格
	fmt.Println(tw.Render())
}

func showFrequentlyIo(ioV string) {

	var isPerformanceSchema int
	DB.Raw("SELECT @@performance_schema").Scan(&isPerformanceSchema)

	if isPerformanceSchema == 0 {
		fmt.Println("performance_schema参数未开启。")
		fmt.Println("在my.cnf配置文件里添加performance_schema=1，并重启mysqld进程生效。")
		return
	}

	DB.Exec("SET @sys.statement_truncate_len = 1024")
	rows, err :=
		DB.Raw("select file,count_read,total_read,count_write,total_written,total from sys.io_global_by_file_by_bytes limit @io", sql.Named("io", ioV)).Rows()
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(rows)
	if err != nil {
		fmt.Println(err)
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
	tw.AppendHeader(table.Row{"表文件名", "总共读取次数", "总共读取数据量", "总共写入次数", "总共写入数据量", "总共读写数据量"})

	for rows.Next() {
		var file, countRead, totalRead, countWrite, totalWritten, total sql.NullString
		err := rows.Scan(&file, &countRead, &totalRead, &countWrite, &totalWritten, &total)
		if err != nil {
			fmt.Println(err)
		}
		tw.AppendRow(table.Row{file.String, countRead.String, totalRead.String, countWrite.String, totalWritten.String, total.String})
	}
	fmt.Println(tw.Render())
}

// showLockSql 显示当前数据库中的锁阻塞信息，并根据kill参数决定是否杀死阻塞的查询。
// 参数:
//
//	kill - 布尔值，指示是否执行KILL命令来终止阻塞的SQL查询。
//
// 无返回值。
func showLockSql(kill bool) {

	rows, err := DB.Raw(`SELECT 
            a.trx_id AS trx_id, 
            a.trx_state AS trx_state, 
            a.trx_started AS trx_started, 
            b.id AS processlist_id, 
            b.info AS info, 
            b.user AS user, 
            b.host AS host, 
            b.db AS db, 
            b.command AS command, 
            b.state AS state, 
            CONCAT('KILL QUERY ', b.id) AS sql_kill_blocking_query
        FROM 
            information_schema.INNODB_TRX a, 
            information_schema.PROCESSLIST b 
        WHERE 
            a.trx_mysql_thread_id = b.id
        ORDER BY 
            a.trx_started`).Rows()

	if err != nil {
		fmt.Println(err)
	}

	// 表格
	tw := table.NewWriter()
	tw.SetTitle("Lock Blocking")
	tw.SetStyle(table.Style{
		Name:    "StyleDefault",
		Box:     table.StyleBoxDefault,
		Color:   table.ColorOptionsDefault,
		Format:  table.FormatOptionsDefault,
		HTML:    table.DefaultHTMLOptions,
		Options: table.OptionsDefault,
		Title:   table.TitleOptions{Align: text.AlignCenter},
	})
	tw.AppendHeader(table.Row{"事务ID", "事务状态", "执行时间", "线程ID", "info", "user", "host", "db", "command", "state", "kill阻塞查询ID"})

	for rows.Next() {
		var trxId, trxState, trxStarted, processlistId, info, user, host, db, command, state, sqlKillBlockingQuery sql.NullString
		err := rows.Scan(&trxId, &trxState, &trxStarted, &processlistId, &info, &user, &host, &db, &command, &state, &sqlKillBlockingQuery)
		if err != nil {
			fmt.Println(err)
		}
		tw.AppendRow(table.Row{trxId.String, trxState.String, trxStarted.String, processlistId.String, info.String, user.String, host.String,
			db.String, command.String, state.String, sqlKillBlockingQuery.String})
	}
	fmt.Println(tw.Render())

	// kill掉被锁住的sql
	if kill {
		kRow, err := DB.Raw(`SELECT 
                a.trx_id AS trx_id, 
                a.trx_state AS trx_state, 
                a.trx_started AS trx_started, 
                b.id AS processlist_id, 
                b.info AS info, 
                b.user AS user, 
                b.host AS host, 
                b.db AS db, 
                b.command AS command, 
                b.state AS state, 
                CONCAT('KILL CONNECTION ', b.id) AS sql_kill_blocking_query
            FROM 
                information_schema.INNODB_TRX a, 
                information_schema.PROCESSLIST b 
            WHERE 
                a.trx_mysql_thread_id = b.id and a.trx_state = 'RUNNING'
            ORDER BY 
                a.trx_started`).Rows()
		if err != nil {
			fmt.Println(err)
		}

		newList := list.NewList([]string{})
		defer func(kRow *sql.Rows) {
			err := kRow.Close()
			if err != nil {
				fmt.Println(err)
			}
		}(kRow)
		for kRow.Next() {
			var trxId, trxState, trxStarted, processlistId, info, user, host, db, command, state, sqlKillBlockingQuery sql.NullString
			err := kRow.Scan(&trxId, &trxState, &trxStarted, &processlistId, &info, &user, &host, &db, &command, &state, &sqlKillBlockingQuery)
			if err != nil {
				fmt.Println(err)
			}
			newList.Push(sqlKillBlockingQuery.String)
		}
		newList.ForEach(func(sqlKill string) {
			DB.Exec(sqlKill)
			fmt.Printf("已成功执行 %s\n", sqlKill)
		})
	}
}

func showRedundantIndexes() {

	var isPerformanceSchema int
	DB.Raw("SELECT @@performance_schema").Scan(&isPerformanceSchema)

	if isPerformanceSchema == 0 {
		fmt.Println("performance_schema参数未开启。")
		fmt.Println("在my.cnf配置文件里添加performance_schema=1，并重启mysqld进程生效。")
		return
	}

	DB.Exec("SET @sys.statement_truncate_len = 1024")
	rows, err :=
		DB.Raw(`select table_schema,table_name,redundant_index_name,redundant_index_columns,
       sql_drop_index from sys.schema_redundant_indexes`).Rows()
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(rows)
	if err != nil {
		fmt.Println(err)
	}
	// 表格
	tw := table.NewWriter()
	tw.SetTitle("Index Analysis")
	tw.SetStyle(table.Style{
		Name:    "StyleDefault",
		Box:     table.StyleBoxDefault,
		Color:   table.ColorOptionsDefault,
		Format:  table.FormatOptionsDefault,
		HTML:    table.DefaultHTMLOptions,
		Options: table.OptionsDefault,
		Title:   table.TitleOptions{Align: text.AlignCenter},
	})
	tw.AppendHeader(table.Row{"数据库名", "表名", "冗余索引名", "冗余索引列名", "删除冗余索引SQL"})

	for rows.Next() {
		var tableSchema, tableName, redundantIndexName, redundantIndexColumns, sqlDropIndex sql.NullString
		err := rows.Scan(&tableSchema, &tableName, &redundantIndexName, &redundantIndexColumns, &sqlDropIndex)
		if err != nil {
			fmt.Println(err)
		}
		tw.AppendRow(table.Row{tableSchema.String, tableName.String, redundantIndexName.String, redundantIndexColumns.String, sqlDropIndex.String})
	}
	fmt.Println(tw.Render())
}

func showConnCount(ip, port string) {

	rows, err :=
		DB.Raw(`SELECT USER, COUNT(*) FROM information_schema.PROCESSLIST 
                      GROUP BY USER ORDER BY COUNT(*) DESC`).Rows()

	defer func(rows2 *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(rows)
	if err != nil {
		fmt.Println(err)
	}

	// 表格IP total number of connections
	tw := table.NewWriter()
	tw.SetTitle("total number of connections")
	tw.SetStyle(table.Style{
		Name:    "StyleDefault",
		Box:     table.StyleBoxDefault,
		Color:   table.ColorOptionsDefault,
		Format:  table.FormatOptionsDefault,
		HTML:    table.DefaultHTMLOptions,
		Options: table.OptionsDefault,
		Title:   table.TitleOptions{Align: text.AlignCenter},
	})
	tw.AppendHeader(table.Row{"连接用户", "数量"})

	cc := int64(0)
	for rows.Next() {
		var user, count sql.NullString
		err := rows.Scan(&user, &count)
		if err != nil {
			fmt.Println(err)
		}
		toInt, _ := convertor.ToInt(count.String)
		cc += toInt
		tw.AppendRow(table.Row{user.String, count.String})
	}
	fmt.Println("1) 连接数总和")
	fmt.Println(tw.Render())
	fmt.Printf("%s:%s 这台数据库，总共有 【%d】 个连接数\n", ip, port, cc)

	// IP addresses
	rowsIp, err := DB.Raw(`SELECT user,db,substring_index(HOST,':',1) AS Client_IP,count(1) AS count
					FROM information_schema.PROCESSLIST 
					GROUP BY user,db,substring_index(HOST,':',1) 
					ORDER BY COUNT(1) DESC`).Rows()
	defer func(rowsIp *sql.Rows) {
		err := rowsIp.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(rowsIp)
	if err != nil {
		fmt.Println(err)
	}
	// 表格IP addresses
	twIp := table.NewWriter()
	twIp.SetTitle("Total number of connections from application-side IP addresses")
	twIp.SetStyle(table.Style{
		Name:    "StyleDefault",
		Box:     table.StyleBoxDefault,
		Color:   table.ColorOptionsDefault,
		Format:  table.FormatOptionsDefault,
		HTML:    table.DefaultHTMLOptions,
		Options: table.OptionsDefault,
		Title:   table.TitleOptions{Align: text.AlignCenter},
	})
	twIp.AppendHeader(table.Row{"连接用户", "数据库名", "应用端IP", "数量"})

	for rowsIp.Next() {
		var user, db, clientIp, count sql.NullString
		err := rowsIp.Scan(&user, &db, &clientIp, &count)
		if err != nil {
			fmt.Println(err)
		}
		twIp.AppendRow(table.Row{user.String, db.String, clientIp.String, count.String})
	}

	fmt.Println()
	fmt.Println("2) 应用端IP连接数总和")
	fmt.Println(twIp.Render())

}

func showTableInfo() {

	DB.Exec("SET sql_mode=(SELECT REPLACE(@@sql_mode,'ONLY_FULL_GROUP_BY',''))")
	rows, err :=
		DB.Raw(` SELECT t.TABLE_SCHEMA as TABLE_SCHEMA, t.TABLE_NAME as TABLE_NAME, t.ENGINE as ENGINE,
            IFNULL(t.DATA_LENGTH/1024/1024/1024, 0) as DATA_LENGTH,
            IFNULL(t.INDEX_LENGTH/1024/1024/1024, 0) as INDEX_LENGTH,
            IFNULL((DATA_LENGTH+INDEX_LENGTH)/1024/1024/1024, 0) AS TOTAL_LENGTH,
            c.column_name AS COLUMN_NAME, c.data_type AS DATA_TYPE, c.COLUMN_TYPE AS COLUMN_TYPE,
            t.AUTO_INCREMENT AS AUTO_INCREMENT, locate('unsigned', c.COLUMN_TYPE) = 0 AS IS_SIGNED 
        FROM information_schema.TABLES t 
        JOIN information_schema.COLUMNS c ON t.TABLE_SCHEMA = c.TABLE_SCHEMA AND t.table_name=c.table_name 
        WHERE t.TABLE_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys') 
        GROUP BY TABLE_NAME 
        ORDER BY TOTAL_LENGTH DESC, AUTO_INCREMENT DESC`).Rows()
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(rows)
	if err != nil {
		fmt.Println(err)
	}
	// 表格
	tw := table.NewWriter()
	tw.SetTitle("Table Size Statistics")
	tw.SetStyle(table.Style{
		Name:    "StyleDefault",
		Box:     table.StyleBoxDefault,
		Color:   table.ColorOptionsDefault,
		Format:  table.FormatOptionsDefault,
		HTML:    table.DefaultHTMLOptions,
		Options: table.OptionsDefault,
		Title:   table.TitleOptions{Align: text.AlignCenter},
	})
	tw.AppendHeader(table.Row{"库名", "表名", "存储引擎", "数据大小(GB)", "索引大小(GB)", "总计(GB)", "主键字段名", "主键字段属性", "主键自增当前值", "主键自增值剩余"})

	for rows.Next() {
		var tableSchema, tableName, engine, dataLength, indexLength, totalLength, columnName, dataType, columnType, autoIncrement, isSigned sql.NullString
		err := rows.Scan(&tableSchema, &tableName, &engine, &dataLength, &indexLength, &totalLength, &columnName, &dataType, &columnType, &autoIncrement, &isSigned)
		if err != nil {
			fmt.Println(err)
		}
		// todo 待定
		residualAutoIncrement := ""
		if strutil.IsBlank(autoIncrement.String) {
			residualAutoIncrement = "主键非自增"
		}
		tw.AppendRow(table.Row{tableSchema.String, tableName.String, engine.String, dataLength.String, indexLength.String, totalLength.String, columnName.String, columnType.String, autoIncrement.String, residualAutoIncrement})
	}
	fmt.Println(tw.Render())
}

func showFpkInfo() {

	rows, err :=
		DB.Raw(`SELECT t.table_schema,
               t.table_name
        FROM information_schema.tables t
        LEFT JOIN information_schema.key_column_usage k
             ON t.table_schema = k.table_schema
                AND t.table_name = k.table_name
                AND k.constraint_name = 'PRIMARY'
        WHERE t.table_schema NOT IN ('mysql', 'information_schema', 'sys', 'performance_schema')
          AND k.constraint_name IS NULL
          AND t.table_type = 'BASE TABLE'`).Rows()
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(rows)
	if err != nil {
		fmt.Println(err)
	}
	// 表格
	tw := table.NewWriter()
	tw.SetTitle("Find out no primary key")
	tw.SetStyle(table.Style{
		Name:    "StyleDefault",
		Box:     table.StyleBoxDefault,
		Color:   table.ColorOptionsDefault,
		Format:  table.FormatOptionsDefault,
		HTML:    table.DefaultHTMLOptions,
		Options: table.OptionsDefault,
		Title:   table.TitleOptions{Align: text.AlignCenter},
	})
	tw.AppendHeader(table.Row{"库名", "表名"})

	for rows.Next() {
		var tableSchema, tableName sql.NullString
		err := rows.Scan(&tableSchema, &tableName)
		if err != nil {
			fmt.Println(err)
		}
		tw.AppendRow(table.Row{tableSchema.String, tableName.String})
	}
	fmt.Println(tw.Render())
}

func showDeadlockInfo() {

	row :=
		DB.Raw(`SHOW ENGINE INNODB STATUS`).Row()

	var t, name, status sql.NullString
	err := row.Scan(&t, &name, &status)
	if err != nil {
		fmt.Println(err)
		return
	}

	re := regexp.MustCompile(`(?s)LATEST DETECTED DEADLOCK.*?WE ROLL BACK TRANSACTION\s+\(\d+\)`)
	match := re.FindString(status.String)

	if strutil.IsNotBlank(match) {
		fmt.Println("------------------------")
		fmt.Println(match)
		fmt.Println("------------------------")
	}
}
