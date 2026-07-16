# Awaken 内置 Skills（Awaken Skills）开发方案

状态：待开发。本文件是一次性实施方案，交付完成后按 AGENTS 规则归档删除，有效事实迁入 `docs/08_技能/` 与桌面工作台文档。

## 1. 目标

1. Awaken Desktop 随安装包内置一组官方 Skills（首批：word / excel / ppt / pdf 四个 Office Skill）。
2. 技能页在「项目 Skills」前新增「Awaken Skills」分组，内置 Skill 不提供删除入口。
3. 内置 Skill 落盘在 `~/.awaken/skills/system-skills/`。
4. 内置 Skill 的加载、激活、调用与现有全局/项目 Skill 完全一致，不新增第二条执行链路。

## 2. 现状事实（已核实的代码锚点）

| 事实 | 位置 |
| --- | --- |
| Core 已有 bundled 来源，仅从 `AWAKEN_BUNDLED_SKILL_DIRS` 读取，无隐式回退 | `src/skills/discovery.go`（`EnvBundledSkillDirs`） |
| bundled 优先级为 2，低于 user(5)/project(7)：同 ID 的用户/项目 Skill 会正常覆盖内置 Skill | `src/skills/types.go`（`sourcePriority`） |
| 扫描器对源根做单层扫描，`<skill-id>/SKILL.md` 为包结构；无 SKILL.md 的子目录会生成 invalid 条目 | `src/skills/loader.go`（`LoadDir`） |
| 删除已 fail-closed：只允许 project/user 来源，其他来源返回 `ErrSkillDeleteSourceUnsupported` | `src/skills/delete.go` |
| 安装 scope 只有 project/global，不可能装进 bundled | `src/skills/install.go`（`ResolveInstallTarget`） |
| `/v1/skills` 已返回 `sourceOrigin` 与 `scope`；bundled 会以 `"bundled"` 透出 | `src/server/handlers_skills.go`（`publicSkillScope`） |
| 技能页现有「项目 Skills / 全局 Skills」两个 Tab；删除按钮以 `isSkillPackageDeletable`（仅 project/user 为 true）控制 | `apps/desktop/src/renderer/ExtensionsView.tsx` |
| Desktop 启动 Core 的 env 注入点 | `apps/desktop/src/main/runtime-supervisor.ts`（`spawnProcess` env 与 `sanitizedRuntimeEnv`） |
| 内置资源随包分发的既有先例：内置宠物 | `apps/desktop/electron-builder.yml`（extraResources `resources/pets`）、`apps/desktop/src/main/pets/builtin-pet-importer.ts` |
| 激活时机器替换 `${AWAKEN_SKILL_DIR}` 为包根绝对路径 | `src/skillruntime/activation.go` |

## 3. 总体设计

数据流（与内置宠物同构）：

```
仓库 apps/desktop/resources/system-skills/{word,excel,ppt,pdf}/
  → electron-builder extraResources 打进安装包（packaged: <resourcesPath>/system-skills）
  → Desktop 启动时同步到 ~/.awaken/skills/system-skills/（Desktop 独占管理，见 §5.2）
  → runtime-supervisor 启动 Core 时注入 AWAKEN_BUNDLED_SKILL_DIRS=~/.awaken/skills/system-skills
  → Core 以 SourceBundled 加载，Catalog/激活/调用走现有统一链路
  → /v1/skills 返回 sourceOrigin=bundled → 技能页「Awaken Skills」分组展示，无删除入口
```

不新增 route、不新增工具、不改 Skill 执行语义。

## 4. 关键决策（已定，开发时不再讨论）

1. **保留目录名**：`system-skills` 位于用户 Skills 根内部，user 源扫描会把它当坏包。在 `LoadDir` 中把目录名 `system-skills` 列为保留名并跳过（连同现有 `.` 前缀跳过逻辑一处实现）。该名字写入技能包契约文档，用户不得用它作为自装 Skill 的包名。
   - 备选方案（放 `~/.awaken/system-skills`，零 Core 改动）已否决：以本方案的落盘路径为准。
2. **覆盖语义**：用户/项目安装同 ID Skill 时覆盖（shadow）内置版本，属现有 MergeSkills 行为，保留不变；内置版本在列表中以 shadowed 状态存在。
3. **同步策略**：`~/.awaken/skills/system-skills` 为 Desktop 独占目录。每次启动比对清单（app 版本 + 内容哈希），不一致则整体替换。用户对该目录的手工修改在下次同步时丢弃——想定制就复制到全局 Skills 改名安装。
4. **UI 术语**：Tab 文案「Awaken Skills」，wire 值沿用 `sourceOrigin="bundled"`，不新造枚举。

## 5. 任务分解

### T1 Core：用户根扫描跳过保留目录（Go，很小）

- `src/skills/loader.go`：`LoadDir` 跳过目录名 `system-skills`（所有源统一跳过即可——bundled 源根本身指向 system-skills 目录内部，其子包不叫这个名字，不受影响）。
- 测试：`src/skills/skills_test.go` 或 `discovery_test.go` 新增用例——用户根下存在 `system-skills/word/SKILL.md` 时，user 源不产生任何条目（不产生 invalid 条目）；bundled 源指向该目录时四个包正常加载为 `SourceBundled`。
- 删除边界已有实现，补一条显式用例：对 bundled 来源调用 `DeleteSkillPackage` 返回 `ErrSkillDeleteSourceUnsupported`（HTTP 层已映射 400 `SKILL_DELETE_SOURCE_UNSUPPORTED`）。

### T2 Desktop main：资源打包、启动同步、env 注入（TS，核心工作量）

1. **资源入库**：内置 Skill 源文件放 `apps/desktop/resources/system-skills/{word,excel,ppt,pdf}/`（内容见 T4）。
2. **electron-builder**：`apps/desktop/electron-builder.yml` extraResources 增加：
   ```yaml
   - from: resources/system-skills
     to: system-skills
     filter: ["**/*"]
   ```
3. **同步器**：新建 `apps/desktop/src/main/system-skills-importer.ts`（参照 `pets/builtin-pet-importer.ts` 的结构与测试风格）：
   - 源路径：packaged 用 `join(process.resourcesPath, "system-skills")`，dev 用 `join(app.getAppPath(), "resources", "system-skills")`；
   - 目标路径：`join(os.homedir(), ".awaken", "skills", "system-skills")`；
   - 清单文件 `.manifest.json` 写在目标目录内（`.` 前缀目录/文件本就被扫描器跳过），记录 app version 与逐文件 sha256；
   - 启动时：清单缺失或不匹配 → 先写临时目录再原子替换（rename），避免半写状态；匹配 → 跳过；
   - 失败降级：同步失败只记日志、不阻塞 Desktop 启动（Core 会因目录缺失而正常地不加载内置 Skill）。
   - 调用点：`apps/desktop/src/main/index.ts` 启动序列中、RuntimeSupervisor 启动 Core **之前**（与 `importBuiltinPets` 相邻位置）。
4. **env 注入**：`runtime-supervisor.ts`：
   - spawn env 增加 `AWAKEN_BUNDLED_SKILL_DIRS: <目标路径>`；
   - `sanitizedRuntimeEnv` 中 delete 继承来的 `AWAKEN_BUNDLED_SKILL_DIRS`（与 `AWAKEN_CONFIG_DIR` 同列，登录 shell 的值不得改变产品身份）。
5. **测试**：同步器单测（首装、升级替换、清单命中跳过、脏目录恢复）；runtime-supervisor 测试补 env 断言。

### T3 Renderer：Awaken Skills 分组（TS，小）

`apps/desktop/src/renderer/ExtensionsView.tsx`：

- `SKILL_VIEW_TABS` 首位插入 `{ id: "awaken", label: "Awaken Skills" }`（`SkillView` 类型同步扩展），顺序：Awaken Skills → 项目 Skills → 全局 Skills。
- 新增 `visibleAwakenSkills = visibleSkills.filter(s => s.sourceOrigin === "bundled")`，从现有 `visibleOtherSkills` 兜底桶中移出。
- 渲染卡片复用 `renderSkillCard`；对 `sourceOrigin === "bundled"` **不渲染**删除按钮（现状是渲染但 disabled，本需求要求不出现），来源标签 `skillSourceLabel` 增加 bundled → 「Awaken 内置」。
- 空态文案：「内置 Skills 未就绪，重启 Awaken 后重试」。
- 安装弹窗 scope 选项保持 project/global 两项不变。
- 测试：分组过滤、bundled 卡片无删除按钮、Tab 顺序。

### T4 内置 Skill 内容入库（内容工作）

把 `~/Desktop/Awaken-Office-Skills` 的四个包修订后放入 `apps/desktop/resources/system-skills/`。必须先完成已评审出的修复项（详见 2026-07-16 审查结论）：

1. 所有 `<本 Skill 包根>` 占位符改为 `${AWAKEN_SKILL_DIR}`（Core 会机器替换）。
2. word/ppt 的 `npm install -g` + `require()` 改为任务目录本地 `npm install`。
3. `pip3 install` 补 PEP 668 说明（优先 uv，退 venv / `--user --break-system-packages`）。
4. 三个脚本的 `find_soffice()` 补 Windows 探测路径（`C:\Program Files\LibreOffice\program\soffice.exe`）；文档注明 Windows 用 `python` 而非 `python3`。
5. 交付清单补「最终文件必须生成在任务工作区（output roots）内再 `artifact_publish`」。
6. LibreOffice/Poppler 等系统依赖引导优先 `dependency_install` 工具。
7. `accept_changes.py` 改为 pandoc `--track-changes=accept` 实现或删除脚本改文档指引。
8. 删除 pdf SKILL 中「supports_native_pdf 判断」句；重写预览稿 README（或不入库）。
9. frontmatter 补 `version` 字段。

### T5 文档同步（与代码同任务落地）

- `docs/08_技能/技能包与加载契约.md`：bundled 行补充——落盘路径、Desktop 同步机制、`system-skills` 保留目录名、覆盖/shadow 语义、不可删除边界。
- `docs/08_技能/安装与激活契约.md`：删除边界补 bundled fail-closed 说明。
- `docs/08_技能/验收规范.md`：新增内置 Skill 验收项（见 T6）。
- 技能页 UI 行为归属的桌面工作台文档：新增「Awaken Skills」分组与无删除入口的产品事实。
- 根 README 能力地图如提及 Skills 能力范围，同步一句「内置官方 Office Skills」。

### T6 验收

自动化（全部在本工作区实际执行）：

- `go test -count=1 ./src/skills/... ./src/server/...` 与全量 `go test ./...`、`go vet ./...`；
- Desktop 单测（含新 importer、ExtensionsView、runtime-supervisor）、typecheck、production build。

真实 Desktop（按根 README 正式入口）：

1. 全新 userData 启动：`~/.awaken/skills/system-skills` 生成，技能页「Awaken Skills」Tab 列出 4 个 Skill，无删除按钮；
2. 「全局 Skills」Tab 不出现 `system-skills` 坏包条目；
3. 真实调用一个内置 Skill（如 excel：生成 xlsx → recalc → `artifact_publish` 出卡片），确认 `${AWAKEN_SKILL_DIR}` 替换为真实路径、`skill_resource_read` 可读 references；
4. 在全局安装同名 `excel` Skill，确认覆盖生效、内置版本 shadowed，删除该全局 Skill 后内置版本恢复；
5. 手工篡改 `system-skills` 内文件后重启，确认被同步还原；
6. 对 bundled Skill 直接调 DELETE `/v1/skills/{id}`，确认 400 `SKILL_DELETE_SOURCE_UNSUPPORTED`；
7. 留存截图证据（技能页三 Tab、调用产物卡片）。

## 6. 边界与不变量

- 内置 Skill 的执行语义、权限、工具可见性与普通 Skill 完全一致；禁止为 bundled 增加任何特权路径。
- `~/.awaken/skills/system-skills` 归 Desktop 所有；Core 只读；用户修改不受保护。
- `system-skills` 为保留包名：user/project 源中同名目录不加载（写入契约文档）。
- 安装、删除 API 的 scope 集合不变（project/global）；bundled 不可通过任何 API 写入或删除。
- UI 只隐藏入口，安全边界在 Core（已存在，勿依赖前端）。

## 7. 风险

- **升级覆盖用户修改**：已定为明确产品行为（§4.3），文档写清即可。
- **Windows 路径**：importer 与 env 注入需在 Windows 实机验证（目前仅 cross-build，属于已知未验证项，随 Windows 实机验收一并做）。
- **首批 Skill 内容质量**：T4 的修复项若不完成，用户首次使用会遇到 npm/pip 真实失败；T4 是 T2 资源入库的前置。

## 8. 建议开发顺序

T1 → T4 → T2 → T3 → T5 → T6。T1/T4 无依赖可并行。
