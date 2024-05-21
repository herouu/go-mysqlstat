## mysqlstat golang语言版本

参考：
https://github.com/hcymysql/mysqlstat


- [x] 实时监控：mysqlstat 可以实时监控 MySQL 服务器的 QPS（每秒查询数）、TPS（每秒事务数）以及网络带宽使用情况等指标。
- [x] 查询分析：它可以展示执行次数最频繁的前N条 SQL 语句，帮助定位查询效率低下的问题，以便进行优化。
- [x] 表文件分析：mysqlstat 可以列出访问次数最频繁的前N张表文件（.ibd），这有助于查找热点表和磁盘使用情况。
- [x] 锁阻塞：工具可以显示当前被锁阻塞的 SQL 语句，帮助识别并解决锁相关的问题。
- [x] 自动杀死当前锁阻塞的SQL
- [x] 死锁信息：mysqlstat 可以提供关于死锁的信息，帮助 DBA 了解并解决死锁问题。
- [x] 索引分析：它可以查找重复或冗余的索引，帮助优化索引使用和减少存储空间的占用。
- [x] 连接数统计：工具可以统计应用端 IP 的连接数总和，有助于了解数据库的连接负载情况。
- [x] 表大小统计：mysqlstat 可以提供库中每个表的大小统计信息，有助于了解表的存储占用情况。
- [x] 快速找出没有主键的表
- [ ] Binlog 分析：它可以在高峰期分析哪些表的 TPS 较高，帮助定位性能瓶颈或优化热点表。
- [ ] 查看主从复制信息：工具可以提供主从复制状态和延迟情况，方便监控和管理主从复制环境。


```bash
NAME:
   go-mysqlstat - MySQL命令行监控工具 - mysqlstat

USAGE:
   go-mysqlstat [global options] command [command options] 

VERSION:
   1.0.0

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --mysql_ip value, -H value        Mysql IP
   --mysql_port value, -P value      Mysql Port
   --mysql_user value, -u value      Mysql User
   --mysql_password value, -p value  Mysql Password
   --top value                       需要提供一个整数类型的参数值，该参数值表示执行次数最频繁的前N条SQL语句
   --io value                        需要提供一个整数类型的参数值，该参数值表示访问次数最频繁的前N张表文件ibd
   --uncommit value                  需要提供一个整数类型的参数值，该参数值表示时间>=N秒的未提交事务的SQL
   --lock                            查看当前锁阻塞的SQL (default: false)
   --kill                            杀死当前锁阻塞的SQL (default: false)
   --index                           查看重复或冗余的索引 (default: false)
   --conn                            查看应用端IP连接数总和 (default: false)
   --tinfo                           统计库里每个表的大小 (default: false)
   --fpk                             快速找出没有主键的表 (default: false)
   --dead                            查看死锁信息 (default: false)
   --binlog                          Binlog分析-高峰期排查哪些表TPS比较高 (default: false)
   --repl                            查看主从复制信息 (default: false)
   --help, -h                        show help
   --version, -v                     print the version

```