# PostgreSQL Schema SQL

本目录是 Lanverse MVP 数据库物理结构的唯一 SQL 实现源。目标是数据库 `lanverse` 的 `public` schema；不创建额外 PostgreSQL schema，也不包含 `CREATE DATABASE`、用户、权限、扩展、测试数据或删除语句。

执行顺序固定为：

1. `01_projects.sql` 至 `20_delivery_versions.sql`：每张表一个文件，定义列、默认值、主键、普通唯一约束和单表 CHECK。
2. `90_foreign_keys.sql`：在全部表建立后添加 53 条跨表外键和循环引用。
3. `91_indexes.sql`：添加 partial unique、查询、租约和未被 PK/UQ 覆盖的 FK 支撑索引。

文件使用显式 `public.<table>`，因此不依赖调用方 `search_path`。所有主键 UUID 由应用生成；全部外键为 `MATCH SIMPLE`、`NOT DEFERRABLE`、`ON UPDATE RESTRICT`、`ON DELETE RESTRICT`。SQL 不使用 ORM、Alembic autogenerate、psql 元命令或环境变量替换。

当前 Requirement/Design/Plan 尚未 accepted，这些文件是可评审的 Design artifact，不授权直接改动用户现有数据库。获准实现后，Alembic `0001_mvp` 只按上述顺序读取并执行这些文件；测试必须使用隔离空库，先验证目标数据库名称，再与 `pg_catalog` exact-set 比较。
