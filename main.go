package main

import (
	"fmt"
	"github.com/duke-git/lancet/v2/datetime"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/urfave/cli/v2"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type MysqlConnect struct {
	ip   string
	pwd  string
	port string
	name string
}

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
	recvMbps        int
	sendMbps        int
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
	}

	app.Action = mysqlStatusMonitor
	app.Version = "1.0.0"
	err := app.Run(os.Args)
	if err != nil {
		return
	}
}

// table.add_column("Time", justify="left", style="cyan")
//    table.add_column("Select", justify="left")
//    table.add_column("Insert", justify="left")
//    table.add_column("Update", justify="left")
//    table.add_column("Delete", justify="left")
//    table.add_column("Conn", justify="left")
//    table.add_column("Max_conn", justify="left")
//    table.add_column("Recv", justify="left")
//    table.add_column("Send", justify="left")

func mysqlStatusMonitor(c *cli.Context) error {
	//
	sigs := make(chan os.Signal, 1)
	//注册信号处理函数
	// Ctrl+C Ctrl+Z
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTSTP)

	go func() {
		prev := calQuota()
		tw := table.NewWriter()
		tw.SetTitle("Real-time Monitoring")
		tw.AppendHeader(table.Row{"Time", "Select", "Insert", "Update", "Delete", "Conn", "Max_conn", "Recv", "Send"})
		tw.SetAutoIndex(true)
		time.Sleep(1 * time.Second)
		fmt.Println(tw.Render())
		count := 1
		for {
			if count%25 == 0 {
				tw.ResetRows()
			}
			current := calQuota()
			c := buildCalQuota(prev, current)
			prev = current
			tw.AppendRow(table.Row{c.currentTime, c.selectPerSecond, c.insertPerSecond, c.updatePerSecond, c.deletePerSecond, c.connCount,
				c.maxConn, c.recvMbps, c.sendMbps})
			fmt.Print("\033[0m\033[2J\033c")
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

func NewMysqlConnect(c *cli.Context) *MysqlConnect {
	return &MysqlConnect{ip: c.String("mysql_ip"),
		pwd: c.String("mysql_port"), port: c.String("mysql_port"),
		name: c.String("mysql_name")}
}

// 指标计算
func calQuota() *Quota {
	return &Quota{selectCount: 1, insertCount: 2, updateCount: 3, deleteCount: 5, conn: 7, recv: 6, maxConn: 10, send: 11}
}

func buildCalQuota(prev *Quota, c *Quota) *CalQuota {
	//计算每秒操作量和网络数据量
	selectPerSecond := c.selectCount - prev.selectCount
	insertPerSecond := c.insertCount - prev.insertCount
	updatePerSecond := c.updateCount - prev.updateCount
	deletePerSecond := c.deleteCount - prev.deleteCount
	recvPerSecond := c.recv - prev.recv
	sendPerSecond := c.send - prev.send
	currentTime := datetime.FormatTimeToStr(time.Now(), "yyyy-MM-dd HH:mm:ss")
	return &CalQuota{selectPerSecond: selectPerSecond,
		insertPerSecond: insertPerSecond,
		updatePerSecond: updatePerSecond,
		deletePerSecond: deletePerSecond,
		recvPerSecond:   recvPerSecond,
		sendPerSecond:   sendPerSecond,
		currentTime:     currentTime,
	}
}
