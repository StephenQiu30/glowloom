"use client";

import { demoNotice } from "@/components/feedback/demo-notice";
import { ProductShell } from "@/components/layout/product-shell";
import { PageHeading } from "@/components/layout/page-heading";
import { Choice } from "@/components/forms/choice";
import { EmptySearch } from "@/components/feedback/empty-search";
import { useState } from "react";
import { Download, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableCell,
  TableHead,
} from "@/components/ui/table";
import { Alert, AlertDescription } from "@/components/ui/alert";

const auditRows = [
  ["10-07 14:31:52", "chendao", "登录", "会话 macOS", "成功"],
  ["10-07 14:22:08", "shen.wan", "改选候选", "镜头 S03-02-02", "成功"],
  ["10-07 13:06:43", "chendao", "更新凭据", "供应商 MiniMax", "测试失败"],
  ["10-07 13:41:17", "lizhou", "登录", "会话 Windows", "密码错误"],
  ["10-07 12:09:03", "shen.wan", "改定稿", "设定 林舟·雨夜", "成功"],
  ["10-07 11:47:55", "chendao", "发布模型版本", "[视频模型 B] v2", "成功"],
  ["10-07 10:30:12", "ali", "禁用账号", "账号 oldpost", "成功"],
  ["10-06 22:14:38", "xiaoman", "登录", "会话 iPad", "已锁定"],
  ["10-06 19:02:21", "chendao", "复制项目", "雾港来信 → 副本", "成功"],
];

export function AuditPage() {
  const [selected, setSelected] = useState(1);
  const [actor, setActor] = useState("操作人：全部");
  const [action, setAction] = useState("动作：全部");
  const [period, setPeriod] = useState("最近 7 天");
  const [project, setProject] = useState("项目：全部");
  const [object, setObject] = useState("对象：全部");
  const visible = auditRows.filter(
    (row) =>
      (actor.endsWith("全部") || actor.includes(row[1])) &&
      (action.endsWith("全部") || action.includes(row[2])) &&
      (object.endsWith("全部") || row[3].includes(object.split("：")[1])),
  );
  const row = auditRows[selected];
  function exportCsv() {
    const blob = new Blob(
      [
        "\uFEFF" +
          [["时间", "操作人", "动作", "对象", "结果"], ...visible]
            .map((row) =>
              row.map((value) => `"${value.replaceAll('"', '""')}"`).join(","),
            )
            .join("\n"),
      ],
      { type: "text/csv;charset=utf-8" },
    );
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "浮光-演示审计日志.csv";
    a.click();
    URL.revokeObjectURL(url);
    demoNotice("演示日志已导出");
  }
  return (
    <ProductShell screen="audit" contextual>
      <PageHeading
        title="审计日志"
        description="记录账号、凭据、模型、生成与选定等关键操作；只读，不可修改。"
      >
        <Button variant="secondary" onClick={exportCsv}>
          <Download data-icon="inline-start" />
          导出 CSV
        </Button>
      </PageHeading>
      <div className="mb-5 flex flex-wrap gap-2">
        <Choice
          value={period}
          onChange={setPeriod}
          options={["最近 7 天", "最近 14 天", "最近 30 天"]}
          label="时间范围"
        />
        <Choice
          value={actor}
          onChange={setActor}
          options={[
            "操作人：全部",
            "操作人：chendao",
            "操作人：shen.wan",
            "操作人：lizhou",
          ]}
          label="操作人"
        />
        <Choice
          value={project}
          onChange={setProject}
          options={["项目：全部", "项目：雾港来信"]}
          label="项目"
        />
        <Choice
          value={object}
          onChange={setObject}
          options={["对象：全部", "对象：会话", "对象：镜头", "对象：供应商"]}
          label="对象"
        />
        <Choice
          value={action}
          onChange={setAction}
          options={[
            "动作：全部",
            "动作：登录",
            "动作：改选候选",
            "动作：更新凭据",
          ]}
          label="动作"
        />
      </div>
      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="overflow-hidden rounded-lg bg-surface-1">
          <Table>
            <TableHeader>
              <TableRow>
                {["时间", "操作人", "动作", "对象", "结果"].map((head) => (
                  <TableHead key={head}>{head}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.map((item) => (
                <TableRow
                  key={item[0]}
                  data-state={item === row ? "selected" : undefined}
                >
                  <TableCell className="py-3 font-mono text-xs">
                    {item[0]}
                  </TableCell>
                  <TableCell className="font-mono text-xs">{item[1]}</TableCell>
                  <TableCell>
                    <Button
                      variant="link"
                      size="inline"
                      onClick={() => setSelected(auditRows.indexOf(item))}
                    >
                      {item[2]}
                    </Button>
                  </TableCell>
                  <TableCell>{item[3]}</TableCell>
                  <TableCell>
                    <span
                      className={
                        item[4] === "成功" ? undefined : "text-destructive"
                      }
                    >
                      {item[4]}
                    </span>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {!visible.length ? <EmptySearch /> : null}
        </div>
        <Card variant="panel">
          <CardHeader>
            <CardTitle>{row[2]}</CardTitle>
            <CardDescription>
              {row[0]} · {row[1]}
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <dl className="grid grid-cols-[50px_1fr] gap-y-3 text-xs">
              <dt className="text-muted-foreground">项目</dt>
              <dd>雾港来信</dd>
              <dt className="text-muted-foreground">对象</dt>
              <dd>{row[3]}</dd>
              <dt className="text-muted-foreground">请求</dt>
              <dd className="font-mono">req_7c1e_a98</dd>
              <dt className="text-muted-foreground">变更</dt>
              <dd>{row[4]}</dd>
            </dl>
            <div className="overflow-hidden rounded-md bg-background font-mono text-[11px]">
              <p className="bg-destructive/10 p-2 text-destructive">
                − selected_candidate: &quot;A&quot;
              </p>
              <p className="bg-surface-3 p-2">
                + selected_candidate: &quot;B&quot;
              </p>
              <p className="p-2 text-muted-foreground">revision: 11 → 12</p>
            </div>
            <Alert variant="warning">
              <TriangleAlert />
              <AlertDescription>
                上次选定的 2 个下游结果已过期。
              </AlertDescription>
            </Alert>
          </CardContent>
        </Card>
      </div>
    </ProductShell>
  );
}
