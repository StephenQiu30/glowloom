# PostgreSQL Schema SQL

本目录是 Lanverse MVP 数据库物理结构的唯一 SQL 实现源。目标是数据库 `lanverse` 的 `public` schema；不创建额外 PostgreSQL schema，也不包含 `CREATE DATABASE`、用户、权限、扩展、测试数据或删除语句。

执行顺序固定为：

`01_projects.sql` 至 `20_delivery_versions.sql` 按外键依赖顺序执行。每张表只有一个 SQL 文件，列、默认值、PK、UQ、FK 和 CHECK 全部直接定义在该表的 `CREATE TABLE` 中；partial unique、查询、租约与 FK 支撑索引紧随其后写在同一个文件。

文件使用显式 `public.<table>`，因此不依赖调用方 `search_path`。所有主键 UUID 由应用生成；全部外键为 `MATCH SIMPLE`、`NOT DEFERRABLE`、`ON UPDATE RESTRICT`、`ON DELETE RESTRICT`。目录不包含公共关系/索引脚本，不使用后置 `ALTER TABLE`、ORM、Alembic autogenerate、psql 元命令或环境变量替换。

当前 Requirement/Design/Plan 尚未 accepted，这些文件是可评审的 Design artifact，不授权直接改动用户现有数据库。获准实现后，Alembic `0001_mvp` 只按上述顺序读取并执行这些文件；测试必须使用隔离空库，先验证目标数据库名称，再与 `pg_catalog` exact-set 比较。
