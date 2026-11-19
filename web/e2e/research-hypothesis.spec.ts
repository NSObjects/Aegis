import { expect, test } from "@playwright/test";

test("a hypothesis can be saved and reopened from its source report", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("工作台访问口令").fill("ci-browser-access-password");
  await page.getByRole("button", { name: "解锁研究工作台" }).click();
  await expect(page.getByRole("heading", { name: "工作台", exact: true })).toBeVisible();

  const response = await page.request.post("/api/workbench/runs", {
    headers: { Origin: new URL(page.url()).origin },
    data: { symbol: "600519.SH", mode: "PORTFOLIO_REVIEW", portfolio: { state: "NOT_PROVIDED" } },
  });
  expect(response.status()).toBe(202);
  const { run_id } = await response.json();
  await page.goto(`/?run=${run_id}`);

  const openHypotheses = async () => {
    await page.getByText("研究假设", { exact: true }).click();
    await expect(page.getByRole("button", { name: "保存假设" })).toBeEnabled();
  };
  await openHypotheses();
  await page.getByLabel("假设", { exact: true }).fill("回购公告后成交量会回升");
  await page.getByLabel("有效期（上海时间）").fill("2099-01-01T18:00");
  await page.getByLabel("失效条件").fill("公司撤回回购公告");
  await page.getByLabel("验证规则").fill("对比公告前后五个交易日的平均成交量");
  await page.getByLabel("支持证据来源（选填）").fill("公司公告");
  await page.getByLabel("支持证据内容（选填）").fill("拟回购股份");
  await page.getByRole("button", { name: "保存假设" }).click();
  await expect(page.getByText("回购公告后成交量会回升", { exact: true })).toBeVisible();

  const later = await page.request.post("/api/workbench/runs", {
    headers: { Origin: new URL(page.url()).origin },
    data: { symbol: "600519.SH", mode: "PORTFOLIO_REVIEW", portfolio: { state: "NOT_PROVIDED" } },
  });
  expect(later.status()).toBe(202);
  const followup = await later.json();
  await expect.poll(async () => {
    const response = await page.request.get(`/api/workbench/runs/${followup.run_id}`);
    return (await response.json()).state;
  }).toBe("COMPLETED");
  await page.getByLabel("后续报告任务 ID").fill(followup.run_id);
  await page.getByRole("button", { name: "记录后验观察" }).click();
  await expect(page.getByText("证据不足", { exact: true })).toBeVisible();

  await page.getByText("错误复盘", { exact: true }).click();
  await page.getByLabel("关联观察").selectOption(followup.run_id);
  await page.getByLabel("错误类型").selectOption("DATA_MISSING");
  await page.getByLabel("复盘原因").fill("缺少目标日成交量");
  await page.getByLabel("支撑证据来源").fill("后续报告");
  await page.getByLabel("支撑证据内容").fill("行情区间未覆盖目标日");
  await page.getByRole("button", { name: "归档复盘" }).click();
  await expect(page.getByText("缺少目标日成交量", { exact: false })).toBeVisible();

  await page.reload();
  await openHypotheses();
  await expect(page.getByText("回购公告后成交量会回升", { exact: true })).toBeVisible();
  await expect(page.getByText("支持 · 公司公告 · 拟回购股份", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "来源报告", exact: true }).last()).toHaveAttribute("href", `/?run=${followup.run_id}`);
  await page.getByText("错误复盘", { exact: true }).click();
  await expect(page.getByText("缺少目标日成交量", { exact: false })).toBeVisible();
  await expect(page.getByRole("link", { name: "后续报告", exact: true })).toHaveAttribute("href", `/?run=${followup.run_id}`);

  await page.getByText("收入研究记忆", { exact: true }).click();
  await page.getByLabel("记忆来源观察").selectOption(followup.run_id);
  await page.getByLabel("关联复盘（选填）").selectOption({ label: "数据缺失 · 缺少目标日成交量" });
  await page.getByLabel("市场阶段（选填）").fill("盘整");
  await page.getByRole("button", { name: "保存到记忆" }).click();
  await expect(page.getByText("已收入研究记忆。")).toBeVisible();

  await page.goto("/?view=history");
  await page.getByText("研究记忆", { exact: true }).click();
  await page.getByLabel("研究论点").fill("回购公告");
  await page.getByRole("button", { name: "检索研究记忆" }).click();
  await expect(page.getByText("找到 1 条记忆")).toBeVisible();
  await expect(page.getByRole("link", { name: "原报告", exact: true })).toHaveAttribute("href", `/?run=${run_id}`);
  await page.getByText("回测反馈", { exact: true }).click();
  await page.getByLabel("回测验证 JSON").setInputFiles({
    name: "archive.json", mimeType: "application/json",
    buffer: Buffer.from(JSON.stringify({ folds: [{ test_through: "20261120" }] })),
  });
  await page.getByRole("button", { name: "核验并附上反馈" }).click();
  await expect(page.getByText("已记录回测反馈。")).toBeVisible();
  await expect(page.getByText("结果标签来源 · 独立测试标签")).toBeVisible();
  page.once("dialog", dialog => dialog.accept());
  await page.getByRole("button", { name: "撤销记忆" }).click();
  await expect(page.getByText("没有符合条件的记忆。")).toBeVisible();

});
