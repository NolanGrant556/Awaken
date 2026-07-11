# macOS 登录钥匙串认证状态失效 QA 案例

## 文档定位

| 项目 | 内容 |
| --- | --- |
| 事件日期 | 2026-07-11 |
| 事件状态 | 已恢复 |
| 影响平台 | macOS 26.5.2（25F84） |
| 影响范围 | Awaken 无法读取多个已保存的模型 API key，模型运行与余额查询被凭据层阻断 |
| 数据状态 | 凭据记录未丢失或清空；重新认证后无需重录即可恢复原 API key |
| 直接观测 | 登录钥匙串条目可枚举，但秘密值读取与完整性验证被系统拒绝；lock/unlock 后恢复 |
| 机制推断 | 登录钥匙串认证状态不一致，强制重新认证刷新了失效状态；置信度高，但不是 Apple 公开根因 |
| 系统内部根因 | 未获 Apple 正式确认；与 macOS 26.4 以后公开报告的登录钥匙串解锁回归高度吻合 |
| 疑似触发条件 | 首次错误紧随显示器睡眠/唤醒出现；当前证据只能证明时间强相关，不能证明必然因果 |

本文是一次仍用于故障分级、证据采集、人工恢复和回归设计的真实 QA 故障证据，不是历史规范或当前状态机的第二权威。规范性事实只以《[架构与恢复契约](./架构与恢复契约.md)》为准，《[复核清单](./复核清单.md)》只引用该契约；本文完整保留事件证据，但不把命令行操作定义为普通用户流程。

本文所列 `security` 命令仅允许 QA 或工程人员在用户知情、交互式参与的情况下使用。正式产品应在 Awaken 内完成错误分类、数据保护和恢复引导，不应要求普通用户自行打开“钥匙串访问”或执行终端命令。

## 一句话结论

2026 年 7 月 11 日，Awaken 在 macOS 26.5.2 上无法读取既有模型凭据。凭据记录仍在，Electron Safe Storage 对应的钥匙串条目元数据也仍可枚举，但 macOS `securityd` 拒绝执行秘密值读取和完整性验证，导致 Awaken 在启动供应商传输前失败。完全关闭 Awaken，并由用户使用登录钥匙串密码完成一次锁定/解锁认证后，无需重新录入 API key，原模型配置恢复正常。本次只能确认用户输入的密码成功解锁了登录钥匙串；QA 不得据此假设登录钥匙串密码和当前 macOS 登录密码始终相同。

## 用户可见现象

故障期间可观察到：

- 运行任务时显示 `Secure credential storage is unavailable on this device`。
- DeepSeek、Kimi 等多个已配置模型同时显示模型密钥不可用。
- 模型配置与掩码信息仍可见。
- 单独重新启动 Awaken 后仍然失败。
- 运行在供应商请求前终止，没有供应商返回的 `401`、余额、限流或模型错误。

这条英文错误不代表“设备没有安全凭据存储”，也不代表“API key 已被删除”。原实现把 Electron/Chromium 从系统安全存储得到的临时认证失败压缩成了统一错误，Renderer 无法看到 macOS 的精确状态码。

## 凭据存储与调用边界

Awaken 的模型 API key 是全局配置，不是会话级或 workspace 级配置。

```text
model-profiles.json
  └─ 保存经 Electron safeStorage 加密后的 API key 密文

macOS 登录钥匙串
  └─ 保存 Electron Safe Storage 用于保护密文的系统密钥

当前实现的运行路径
  Renderer → Electron 主进程 → safeStorage 解密
           → 本机 awaken-core 内存 → 供应商 Authorization
```

当前配置文件位于：

```text
$HOME/Library/Application Support/@awaken/desktop/config/model-profiles.json
```

重要区别：

- 配置文件中的密文存在，不等于当前系统一定能解密。
- 钥匙串条目可以枚举，不等于 `securityd` 当前允许读取它的秘密值。
- safeStorage 解密失败时，Awaken 不应向供应商发起请求。
- QA 不得把密文、明文 key、掩码尾号或 `Authorization` 请求头加入日志和缺陷附件。

当前实现会让明文 API key 在可信本机 Electron 主进程中短暂存在，并被复制到 `awaken-core` 的 `AgentConfig`。由于 `RunService` 会按 thread 缓存 Agent，明文至少会在对应 thread/Agent 生命周期内存中存在，直到 profile 重建、线程淘汰或进程退出；它不得进入 Renderer、持久化文件、日志或 transcript。本次已同步修订《架构与恢复契约》的文字边界，但清理策略与验证仍是待完成的安全生命周期工作。

## 契约修订与实现状态

本案例促成了相邻《架构与恢复契约》和《复核清单》的同步修订，但文档契约更新不表示对应产品状态、UI 或清理机制已经实现：

1. **已确认待实现的目标恢复契约**在 `relaunch_required` 与 `reentry_required` 之间新增 `system_reauthentication_required` 或等价状态；当前代码尚无该状态。本案例证明 Awaken relaunch 后仍失败时，原凭据仍可能经系统重新认证恢复。
2. 对正在取证的事故，QA 必须先保护旧凭据并完成系统层/应用层分类；只有确认旧凭据永久不可恢复，且安全写入当前可用时，正式产品才可以引导重新录入。
3. 明文 key 的可信内存边界已按当前真实实现修订为 Electron 主进程和 `awaken-core` Agent/thread 生命周期；后续仍需实现并验证明确的清理策略。

在对应实现和自动化验收落地前，QA 处置规则是：**relaunch 后仍失败，不自动删除、清空、覆盖或重录；先完成本文的安全分类。**

## 已确认时间线

以下时间均为本机时间（UTC+8）。证据编号只描述本次调查中使用的证据类别；原始系统输出可能包含用户名、路径或其他隐私，不进入仓库。后续缺陷单应保存对应的脱敏片段。

| 时间 | 事件 | 可直接得出的结论 | 证据编号 |
| --- | --- | --- | --- |
| 2026-07-01 | 历史 `@awaken/desktop Safe Storage` 条目已存在 | 历史 Safe Storage identity 存在 | E-01 钥匙串条目元数据 |
| 2026-07-02 | 当前 `Awaken Safe Storage` 条目已存在 | 当前 identity 存在；双 namespace 是迁移风险 | E-01 钥匙串条目元数据 |
| 2026-07-08 | 安装 macOS 26.5.2（25F84） | 这是事故环境背景，不是立即触发证据 | E-02 系统版本与安装记录 |
| 2026-07-10 晚间 | Awaken 仍可使用原凭据完成真实模型运行 | 系统更新后并未立即失效 | E-03 Awaken 运行记录 |
| 2026-07-11 04:30:36 | 系统日志记录显示器重新点亮 | 与首次钥匙串异常时间高度接近 | E-04 电源/唤醒日志 |
| 2026-07-11 04:30:38 | `securityd`/`secd` 在同一时间窗分别记录“已解锁”和“元数据密钥仍被视为锁定或无法解密”，并返回 `CSSMERR_CSP_INVALID_DATA` | 直接确认同一时间窗出现两类消息；它们属于同一认证状态冲突是高置信推断 | E-05 `securityd`/`secd` 脱敏日志 |
| 2026-07-11 09:52—10:11 | 多个不同 Electron 进程读取时均出现 `CSSMERR_CSP_OPERATION_AUTH_DENIED` 与完整性检查拒绝 | 单独重启 Awaken 没有消除该故障 | E-06 跨 PID 脱敏日志 |
| 2026-07-11 13:07 | 完全关闭 Awaken 后锁定登录钥匙串，并由用户交互式输入登录钥匙串密码成功解锁 | lock/unlock 操作成功；“刷新认证上下文”是机制推断 | E-07 终端退出状态与状态检查 |
| 2026-07-11 13:10—13:11 | 原 profile 在未重新录入 API key 的情况下完成真实模型运行 | 原凭据语义未丢失，恢复成功 | E-08 真实 Desktop 运行记录 |

双 Safe Storage namespace 是需要治理的历史迁移风险，但它早于本次故障存在，且不能解释大量非 Awaken 钥匙串项目也同时认证失败，因此不是本次突然失效的直接原因。

## 证据与根因边界

### 已确认

1. `model-profiles.json` 内的凭据记录没有丢失或清空，且原 API key 无需重新录入即可恢复；没有结构化写入证据时，不声称密文字节从未变化。
2. `Awaken Safe Storage` 与历史 `@awaken/desktop Safe Storage` 条目元数据仍存在。
3. 登录钥匙串能枚举项目，但受保护数据读取返回 `errSecAuthFailed (-25293)`。
4. `securityd` 出现 `CSSMERR_CSP_INVALID_DATA`、`CSSMERR_CSP_OPERATION_AUTH_DENIED` 和完整性检查拒绝。
5. 同期受影响的不只有 Awaken 条目，Apple 签名的系统进程也发生钥匙串读取失败。
6. `/Library/Keychains/System.keychain` 正常，故障集中在当前用户的登录钥匙串。
7. 多次重启 Awaken 进程后仍失败，说明异常状态不只存在于 Awaken 进程内。
8. 用户交互式输入能够成功解锁登录钥匙串的密码后，原 API key 无需重新录入即可继续使用。
9. 故障发生在 safeStorage 解密阶段，LLM 供应商没有收到请求。

### 高置信推断

- 睡眠/唤醒后，macOS 进入了“逻辑上已解锁，但主密钥或元数据密钥认证上下文不可用”的不一致状态。
- `securityd` 持有的系统级认证状态不会随 Awaken/Electron 进程重启而重建。
- 强制锁定并由当前用户交互式重新解锁，刷新了该认证上下文。
- 本机现象与 Apple Developer Forums 中 macOS 26.4 以后“项目存在、认证操作失败、进程重启无效、锁定/解锁后恢复”的报告高度一致。

### 尚未证实

- 不能写成“Apple 已确认 macOS 26.5.2 存在这个确切 Bug”。Apple DTS 只说明 macOS 26.4 对登录钥匙串解锁有显著变化，并建议提交系统 Bug；没有公开内部根因。
- 不能写成“睡眠/唤醒必然触发”。目前仅确认首次系统错误紧随显示器唤醒出现，尚未建立稳定复现率。
- 不能断言是哪一个 Apple 内部缓存、句柄或密钥对象损坏。
- 不能把日志中的 iCloud Keychain 解锁失败直接等同于 Awaken 使用的具体钥匙串条目。
- 不能把本次事件归因于 Electron 临时升级；事故前后 Electron 版本没有变化，但这也不代表 Electron 的错误分类无需改进。
- 不能把双 Safe Storage namespace、开发版签名或应用名称变化认定为本次直接触发因素；这些属于独立的持久化身份风险。

### 本次可以排除的误判

| 误判 | 为什么不成立 |
| --- | --- |
| API key 被 Awaken 删除 | 凭据记录仍存在，重新认证后无需重录即可恢复原 API key；这证明语义可恢复，不要求密文字节永远不轮换 |
| 用户刚修改了账户登录密码 | 没有近期账户密码修改证据，且本次输入能够成功解锁登录钥匙串；但不能据此假设账户登录密码和登录钥匙串密码始终同步 |
| LLM 供应商拒绝了 key | 失败发生在本地解密阶段，请求没有到达供应商 |
| 只有 Awaken 的 ACL 出错 | 大量非 Awaken 项目及系统进程也发生认证失败 |
| 钥匙串永久损坏 | 一次受控重新认证后原数据恢复可读，不支持永久损坏结论 |
| 设备不支持安全存储 | 同一设备此前和恢复后均可正常使用 safeStorage |

## 错误文案的正确解释

### `Secure credential storage is unavailable on this device`

在本案例中，它表示 Electron/Chromium 没能从 macOS Keychain 取得解密所需的保护密钥。它是通用产品错误，不足以区分：

- 系统凭据存储暂时不可用；
- 用户取消了系统认证；
- 登录钥匙串认证状态异常；
- Safe Storage identity 或历史密文不匹配；
- 密文永久不可恢复。

QA 必须结合系统状态和恢复结果分类，不能只凭这句话要求用户重新录入 key。

### `The user name or passphrase you entered is not correct.`

`security show-keychain-info` 在本次事故中返回过这条通用文案，对应 `errSecAuthFailed (-25293)`。该检查没有提交用户密码，因此这句话不是“用户输入了错误密码”的证据。只有在 `security unlock-keychain` 交互式认证时，系统才实际验证用户输入的密码。

## QA 分级诊断流程

```text
Awaken 显示模型密钥不可用
  │
  ├─ 配置文件 missing/invalid，或凭据记录数为 0
  │    └─ 转数据、userData 与应用 identity 调查；不直接认定“未配置”
  │
  └─ 密文记录仍存在
       │
       ├─ auth_failed，或已有认证/完整性拒绝证据
       │    └─ 满足安全准入 → 完全关闭 Awaken → 用户参与的 lock/unlock
       │          │
       │          ├─ 解锁后原 profile 恢复 → 系统认证上下文暂态失效
       │          └─ 解锁失败或仍不可读 → 停止操作，升级系统凭据故障
       │
       ├─ other_error、路径/权限错误或未知错误
       │    └─ 停止调查，不执行 lock/unlock
       │
       └─ keychain_info=ok，但 Awaken 仍失败
            └─ 转查单条目 ACL/用户取消/provider 状态，以及应用 identity、namespace、签名和密文兼容
```

诊断顺序必须是“保留数据 → 判断系统层或应用层 → 受控恢复 → 用原凭据验收”。不得先删除条目或重录 key，否则会破坏根因证据并覆盖可恢复数据。

## 安全诊断步骤

### 前置条件

- 使用发生故障的同一 macOS 登录用户。
- 如果无法确认当前用户就是保存凭据时的用户，停止人工恢复并转应用 identity/用户目录调查。
- 先确认没有任何人正在保存或更新模型 API key。
- 不采集明文 key、密文、掩码尾号或网络授权头。
- 任何命令原始输出都不得直接粘贴到缺陷单；只填写本文定义的枚举状态。确需保留片段时先去除用户名、UID、主目录、条目 account/label 和进程参数。
- 需要执行锁定/解锁时，先保存 Awaken 和其他应用中的工作，完全关闭 Awaken，并告知用户该操作会影响浏览器、密码管理器等依赖登录钥匙串的应用。
- 在锁定前，用户必须确认知道登录钥匙串密码。它通常与 macOS 登录密码相同，但在密码被管理员重置等情况下可能不同；无法确认时不得执行。
- 推荐在专用 QA 账号或测试机复现；不得把主动锁定登录钥匙串加入普通用户机器上的自动化 smoke。

### 1. 记录系统基线

```bash
/usr/bin/sw_vers
```

记录 `ProductVersion` 与 `BuildVersion`。同时记录 Awaken commit、Electron 版本、打包/开发态、故障时间、最近一次睡眠/唤醒时间，但不要在缺陷附件中包含用户名或主目录绝对路径。

### 2. 只检查登录钥匙串状态

```bash
/usr/bin/security show-keychain-info "$HOME/Library/Keychains/login.keychain-db"
```

正常状态可归一化记录为 `keychain_info=ok`；不要复制包含个人路径的原始输出。其形式类似：

```text
Keychain ".../login.keychain-db" no-timeout
```

异常示例可能是 `errSecAuthFailed (-25293)` 对应的通用认证文案。不要把该文案记录为“用户密码错误”。

### 3. 只检查 Safe Storage 条目元数据是否存在

以下命令不加 `-w` 或 `-g`，只用于确认条目元数据：

```bash
/usr/bin/security find-generic-password \
  -s "Awaken Safe Storage" \
  "$HOME/Library/Keychains/login.keychain-db"
```

如需验证历史 identity：

```bash
/usr/bin/security find-generic-password \
  -s "@awaken/desktop Safe Storage" \
  "$HOME/Library/Keychains/login.keychain-db"
```

条目存在只能证明记录未被删除，不能证明秘密值当前可读。命令还会输出路径和条目属性，只能归一化记录为 `current_metadata=present|absent|error` 与 `legacy_metadata=present|absent|error`，不得上传原始输出，也不得由两个条目都存在推断当前构建实际使用哪一个 identity。

### 4. 只检查模型配置中是否有密文记录

以下命令使用项目必需的 Node.js，仅输出文件状态、profile 总数、凭据记录数、envelope 版本和 scheme 的去重列表；不输出 profile ID、`encryptedApiKey` 或 `last4`：

```bash
node <<'NODE'
const fs = require("node:fs");
const path = require("node:path");

const file = path.join(
  process.env.HOME,
  "Library/Application Support/@awaken/desktop/config/model-profiles.json",
);
if (!fs.existsSync(file)) {
  console.log({ configFileStatus: "missing" });
  process.exit(0);
}

try {
  const parsed = JSON.parse(fs.readFileSync(file, "utf8"));
  const profiles = Array.isArray(parsed.profiles) ? parsed.profiles : [];
  const credentials = profiles
    .map((profile) => profile.credential)
    .filter((credential) => credential?.encryptedApiKey);
  const versionClass = (credential) => {
    if (credential.version === undefined || credential.version === 1) return "1";
    if (credential.version === 2) return "2";
    return "other";
  };
  const schemeClass = (credential) => {
    if (credential.scheme === undefined) return "legacy";
    if (credential.scheme === "electron-safe-storage") {
      return "electron-safe-storage";
    }
    return "other";
  };

  console.log({
    configFileStatus: "readable",
    profileCount: profiles.length,
    credentialRecordCount: credentials.length,
    envelopeVersions: [...new Set(credentials.map(versionClass))],
    schemes: [...new Set(credentials.map(schemeClass))],
  });
} catch {
  console.log({ configFileStatus: "invalid_or_unreadable" });
}
NODE
```

该路径是当前开发态默认 `userData` 位置。文件为 `missing` 时，先核对当前渠道的实际 `userData` 与应用 identity；不得直接归类为“用户没有 key”。如果实际 schema 已升级，QA 应同步更新只输出聚合状态的查询，不得为了适配而打印完整文件。

聚合结果只能证明凭据记录存在，不能证明密文可解、内容正确或在故障前后逐字节未变。人工事件验收以“无需重录即可恢复原 profile”证明凭据语义仍可恢复；是否发生写入应由结构化遥测或自动化断言 `credential_write_attempted=false`。若当前版本没有该证据，缺陷单必须填写 `credential_write_attempted=unknown`，不得猜测。成功解密后的安全轮换可能合法改变密文，因此“逐字节不变”本身也不是产品不变量。

### 5. 可选采集 `securityd` 脱敏日志

```bash
/usr/bin/log show --last 2h --style compact \
  --predicate 'process == "securityd" AND (
    eventMessage CONTAINS[c] "CSSMERR_CSP_INVALID_DATA" OR
    eventMessage CONTAINS[c] "CSSMERR_CSP_OPERATION_AUTH_DENIED" OR
    eventMessage CONTAINS[c] "error while checking integrity" OR
    eventMessage CONTAINS[c] "cannot decrypt metadata key"
  )'
```

上传前去除用户名、UID、路径、进程参数及任何无法确认是否包含敏感信息的上下文。只保留时间、状态码、进程名和必要错误类别。

## 安全恢复步骤

本节是工程/QA 的人工恢复流程，不是正式产品 UX。

只有同时满足以下条件才允许进入本节：

- 登录钥匙串检查已归类为 `keychain_info=auth_failed`，或已有脱敏日志确认同类认证/完整性拒绝，且凭据记录仍存在；`other_error`、路径错误、权限错误或未知错误必须停止调查，不得进入 lock/unlock；
- 用户明确同意该操作，并确认知道登录钥匙串密码；
- Awaken 与其他依赖钥匙串的应用均已保存工作；
- 终端录屏、会话记录和远程协助已暂停，密码提示区域不会被采集；
- QA 已理解：lock 成功但 unlock 失败会让登录钥匙串保持锁定，必须立即转系统管理员或 Apple 支持，不能继续猜密码。

### 1. 完全关闭 Awaken

在仓库根目录按 `README.md` 的正式生命周期入口执行：

```bash
npm run close:desktop
```

该托管关闭命令会清理 Electron、renderer 与 `awaken-core`，必要时可能强制终止未及时退出的受管进程，因此必须先保存工作。只有命令成功返回并报告 Desktop 已关闭才继续；失败时停止，不自行对不明 PID 执行 `kill`。单独关闭一个窗口或只结束 renderer 不满足前置条件。

### 2. 锁定登录钥匙串

再次向用户确认：登录钥匙串密码可能不等于当前 macOS 登录密码。确认无误后，由当前登录用户执行：

```bash
/usr/bin/security lock-keychain "$HOME/Library/Keychains/login.keychain-db"
```

只有命令以退出码 `0` 成功返回，才进入下一步。lock 失败时停止，不执行其他钥匙串写操作。

### 3. 交互式解锁登录钥匙串

由用户本人执行：

```bash
/usr/bin/security unlock-keychain "$HOME/Library/Keychains/login.keychain-db"
```

命令会在终端中交互式请求登录钥匙串密码，输入不回显。QA、日志、自动化、录屏与远程协助不得获取密码。

只有命令以退出码 `0` 成功返回，才继续。若返回非零：

1. 立即停止，不反复猜测密码；
2. 明确告知用户登录钥匙串当前可能保持锁定；
3. 不重置钥匙串、不删除 Safe Storage 条目、不重录 API key；
4. 将事件归类为“登录钥匙串解锁失败”，转系统管理员或 Apple 支持处理。

禁止使用：

```text
/usr/bin/security unlock-keychain -p "<password>" ...
```

`-p` 会把密码暴露给 shell 历史、进程参数或日志。

### 4. 验证系统状态

```bash
/usr/bin/security show-keychain-info "$HOME/Library/Keychains/login.keychain-db"
```

只有该检查不再返回认证错误，才进入 Awaken 验收。原始输出不得上传，只记录 `keychain_info=ok|auth_failed|other_error`。

### 5. 按正式入口启动 Awaken

```bash
npm run open:desktop
```

不要先更新、重录或删除 API key。先使用事故前的原 profile 进行真实运行，才能证明旧数据是否可恢复。

### 6. 用原凭据完成真实 Desktop 验收

按顺序验证：

1. 模型配置仍存在，且不再显示安全存储不可用。
2. 余额查询能够越过凭据层；供应商自身不支持余额查询时，记录其真实业务结果。
3. 在获得调用授权并确认费用边界后，使用原 profile 发送固定、无业务敏感内容的最小真实 prompt。
4. 分别记录 `credential_decrypted`、`transport_started` 和 `provider_response_received`；不能由普通网络错误推断供应商已经收到请求。
5. 确认凭据记录没有因失败流程被删除或改成空值；如有安全轮换，只按“新密文成功后替换旧密文”的契约另行验收。
6. 确认 Renderer、日志、transcript 和错误提示均未出现明文 key、密文或底层系统错误详情。

## 恢复结果分类

不要用一个错误文案直接命名根因。先记录五个互相独立的事实轴；没有证据时填写 `unknown`：

| 事实轴 | 允许值 |
| --- | --- |
| `credential_record` | `present` / `absent` / `unknown` |
| `keychain_info` | `ok` / `auth_failed` / `other_error` / `not_applicable` / `unknown` |
| `system_reauthentication` | `not_run` / `succeeded` / `failed` |
| `credential_decrypted` | `yes` / `no` / `unknown` |
| `transport` | `not_started` / `started_no_response` / `provider_response_received` / `unknown` |

再按下表只选择一个主分类：

| 主分类 | 最低证据门槛 | 后续 |
| --- | --- | --- |
| A. 配置缺失或损坏 | 配置文件 `missing`/`invalid_or_unreadable`，或凭据记录数为 0 | 转数据与 `userData`/identity 调查；不能归因于 Keychain |
| B. 系统认证暂态失效，已恢复 | relaunch 无效；`keychain_info=auth_failed` 或有同类认证/完整性错误；lock/unlock 成功；未重录即 `credential_decrypted=yes` | 本案例同类。记录系统层恢复成功，不把机制推断写成 Apple 已确认根因 |
| C. 登录钥匙串解锁失败 | lock 成功后，交互式 unlock 返回非零 | 立即停止；钥匙串可能仍锁定，转系统管理员或 Apple 支持 |
| D. 系统凭据持久故障 | unlock 成功但 `keychain_info` 仍异常 | 禁止重置或删除，转系统管理员或 Apple 支持 |
| E. 应用或条目级解密故障待查 | `keychain_info=ok`，但 Awaken 仍 `credential_decrypted=no` | 总体状态正常不能排除单条目 ACL、用户取消或临时 provider 状态；同时转查 app name、bundle ID、签名、Safe Storage namespace、`userData` 与 envelope 兼容 |
| F. 供应商凭据拒绝 | `provider_response_received`，且供应商明确返回 `401`/等价鉴权错误 | 请求已越过安全存储，按供应商凭据问题处理 |
| G. 供应商业务错误 | `provider_response_received`，且返回限流、余额、模型权限等业务错误 | 与钥匙串故障分开归类 |
| H. 传输错误 | `credential_decrypted=yes`、`transport=started_no_response` | DNS、代理、TLS、超时等只证明本地启动传输；供应商是否收到为未知 |

本事故在 safeStorage 解密失败时属于 `transport=not_started`，因此可以确认 Awaken 没有向供应商发出该次请求。其他网络错误不能沿用这个结论。

## 禁止操作

以下操作会泄露秘密、破坏证据或让可恢复数据变成永久丢失：

- 禁止使用 `/usr/bin/security find-generic-password -w` 或 `-g`。
- 禁止使用 `/usr/bin/security dump-keychain`。
- 禁止把密码写入 `/usr/bin/security unlock-keychain -p ...`。
- 禁止执行 `/usr/bin/security delete-generic-password`。
- 禁止删除 `Awaken Safe Storage` 或 `@awaken/desktop Safe Storage` 条目。
- 禁止重置默认钥匙串作为第一恢复步骤。
- 禁止打印、上传或附加完整 `model-profiles.json`。
- 禁止打印 `encryptedApiKey`、`last4` 或任何密文指纹；布尔存在性足够。
- 禁止在读取失败时自动清空、覆盖或重新生成 API key。
- 禁止为了验证而先让用户重录 key；这会破坏“原密文能否恢复”的关键证据。
- 禁止降级为明文文件、环境变量回写、session-only 或 workspace 级临时凭据。
- 禁止采集可能包含 `Authorization`、`x-api-key` 或请求正文的网络日志。
- 禁止把 `errSecAuthFailed (-25293)` 直接写成“用户输错密码”。
- 禁止把 lock/unlock 命令设计成普通自动化 smoke；它会影响该登录钥匙串中的其他应用。
- 禁止直接上传本文任何诊断命令的原始输出；缺陷单只收标准化枚举与脱敏片段。

## 回归测试矩阵

### macOS 与通用凭据层

下表是覆盖要求，不是授权 QA 在个人或生产账号中随意制造故障。MAC-02、MAC-06、MAC-07 只有在可销毁 QA 用户/机器、已评审的故障注入方案和完整回滚步骤齐备后才能执行；方案缺失时标记 `blocked`，不得临场修改或重置密码。

| 编号 | 场景 | 前置/操作 | 核心断言 | 优先级 |
| --- | --- | --- | --- | --- |
| MAC-01 | 正常保存与完整重启 | 保存测试 key，按正式入口关闭并重新启动 | 原 profile 可读，真实 prompt 成功 | P0 |
| MAC-02 | 登录钥匙串暂不可读 | 仅在隔离 QA 账号中制造受控不可读状态 | UI 进入可恢复状态；不删除、不覆盖；`transport=not_started` | P0 |
| MAC-03 | 故障后多次重启 Awaken | 在系统状态未恢复时重复受控 relaunch | 凭据记录不删除/清空，`credential_write_attempted=false`，不产生明文 fallback | P0 |
| MAC-04 | 系统重新认证后恢复 | Awaken 完全退出，用户执行 lock/unlock | 无需重录 key，原 profile 恢复 | P0 |
| MAC-05 | 睡眠/唤醒循环 | 保存凭据后进行锁屏、睡眠、显示器唤醒和整夜待机 | 成功读取，或稳定分类为可恢复状态；无数据覆盖 | P1 |
| MAC-06 | 正常修改登录密码 | 通过系统设置修改密码并重新登录 | 同一用户下原凭据仍可读，或给出可诊断恢复状态 | P1 |
| MAC-07 | 管理员重置密码 | 在可销毁 QA 账号中模拟旧钥匙串密码不可得 | 进入重新录入状态；旧密文不自动删除，不明文回退 | P1 |
| MAC-08 | 历史/当前 namespace | 覆盖 `@awaken/desktop` 与 `Awaken` 两种 Safe Storage identity | 新密文成功持久化后才替换；失败保留旧数据 | P0 |
| MAC-09 | 打包与开发身份 | 比较开发态、正式签名包和版本升级 | app name、bundle ID、签名身份、userData 路径稳定 | P1 |
| CORE-01 | 所有业务入口统一分类 | 运行、余额、模型测试、保存分别触发同类错误 | 使用同一凭据层、状态码与脱敏提示 | P0 |
| CORE-02 | 安全写入失败 | 更新 key 时模拟 secure write 失败 | 明确显示“未保存”，新 key 不落盘，旧密文不变 | P0 |
| CORE-03 | legacy 与轮换 | legacy 密文或 `shouldReEncrypt=true` | 新密文成功并原子持久化后才替换旧密文 | P0 |
| CORE-04 | 保存与自动迁移并发 | 迁移期间用户更新同一 profile | 用户新保存结果不被旧迁移覆盖 | P0 |
| CORE-05 | 供应商错误区分 | 分别模拟本地解密失败、`401`、限流、余额与网络错误 | UI/遥测区分 `credential_decrypted`、`transport_started`、`provider_response_received` | P0 |
| SEC-01 | 敏感信息审计 | 检查 IPC、日志、toast、余额、transcript、崩溃报告 | 无明文 key、密文、掩码尾号与原始系统异常 | P0 |

### Windows 对应回归

本事件的具体系统故障只发生在 macOS Keychain。Windows 使用 DPAPI，Windows QA 不执行本文“安全诊断步骤”和“安全恢复步骤”中的任何钥匙串命令；两端共享的是 Awaken 的产品状态机、数据保留和错误脱敏契约，而不是底层恢复命令。Windows 用例必须有独立执行手册后才能实际执行。

| 编号 | 场景 | 核心断言 | 优先级 |
| --- | --- | --- | --- |
| WIN-01 | DPAPI 保存后完整重启 | 同一用户下原 key 可解密并调用供应商 | P0 |
| WIN-02 | 锁屏、睡眠与恢复 | 自动恢复或进入统一可恢复状态；不覆盖密文 | P1 |
| WIN-03 | 用户正常修改密码 | 同一用户下原凭据仍可读 | P1 |
| WIN-04 | 管理员重置密码 | 不可恢复时进入应用内重新录入；旧密文不自动删除 | P1 |
| WIN-05 | 跨用户或跨机器复制 userData | 明确判定不可恢复；不明文回退，不误报 provider 错误 | P1 |
| WIN-06 | DPAPI 写入失败 | 新 key 不落盘，旧密文保持不变 | P0 |
| UX-01 | 跨平台相同产品状态 | 正式 UI 不暴露 Keychain、DPAPI 或终端操作术语 | P0 |

## 缺陷证据采集模板

复制以下模板到缺陷单；所有敏感字段只记录布尔值、计数或分类，不记录实际内容。命令原始输出一律不上传。

```markdown
### 环境

- 发生时间：
- macOS/Windows 版本与 build：
- Awaken commit/版本：
- Electron 版本：
- 开发态/签名安装包：
- 当前登录用户是否与保存凭据时相同：是/否/未知（未知时停止人工恢复）
- 最近系统升级时间：
- 最近睡眠/唤醒或锁屏时间：

### 用户现象

- 受影响入口：运行/余额/模型测试/保存
- 受影响 profile 数：
- UI 错误分类：
- Awaken relaunch 后是否恢复：是/否

### 数据状态（不得粘贴内容）

- configFileStatus：readable/missing/invalid_or_unreadable
- profileCount：
- credentialRecordCount：
- Safe Storage 元数据：current=present/absent/error；legacy=present/absent/error
- credential_write_attempted：true/false/unknown
- 是否曾重新录入 key：是/否

### macOS 系统状态（Windows 填 N/A）

- keychain_info：ok/auth_failed/other_error/unknown
- 相关系统错误码：
- 非 Awaken 钥匙串项目是否同时失败：是/否/未检查
- lock：succeeded/failed/not_run
- unlock：succeeded/failed/not_run
- 原始证据编号：

### 端到端结果

- credential_decrypted：yes/no/unknown
- transport：not_started/started_no_response/provider_response_received/unknown
- 使用原 profile 是否恢复：是/否
- 真实 prompt 结果：成功/provider 业务错误/transport 错误/凭据层失败

### 附件脱敏确认

- [ ] 无明文 API key
- [ ] 无 encryptedApiKey、last4 或完整配置文件
- [ ] 无 Authorization/x-api-key 请求头
- [ ] 无用户密码、用户名或个人主目录
- [ ] 未附任何诊断命令原始输出；引用片段已二次脱敏
```

## QA 通过标准

一次同类事件只有同时满足以下条件才能关闭：

- [ ] 凭据记录没有被删除或清空，且无需重录即可恢复原 profile；如声称故障期间未写入，已有 `credential_write_attempted=false` 的结构化证据。
- [ ] 已区分“系统安全存储不可读”和“供应商拒绝 API key”。
- [ ] 恢复过程中没有读取、打印或复制任何秘密值。
- [ ] 系统重新认证后，没有重新录入 key 即可读取原 profile。
- [ ] 已分别记录 `credential_decrypted`、`transport` 与供应商响应；没有把 DNS、代理、TLS 或超时误写成供应商已收到。
- [ ] 恢复验收的真实 Desktop prompt 已取得供应商响应或明确业务错误；如因 transport 故障无法完成，凭据恢复只记为部分通过。
- [ ] 运行、余额、测试与保存使用一致的错误分类。
- [ ] UI、IPC、日志和 transcript 没有泄露明文 key、密文或底层原始错误。
- [ ] macOS 特定恢复步骤没有被写进跨平台常规产品 UX。
- [ ] 如仍失败，已按“系统层 / 应用 identity 与迁移层 / provider 层”分派，而不是删除数据后重试。

## 产品改进项

本案例说明“重新启动 Awaken”只能重建 Electron 的 safeStorage provider，无法保证刷新 `securityd` 的系统级认证上下文。后续产品与工程工作应覆盖：

1. 凭据层输出结构化状态：`temporary_unavailable`、`relaunch_required`、`system_reauthentication_required`、`reentry_required`，不再只返回通用英文异常。
2. 第一次临时失败可执行受控 relaunch；同类错误跨进程持续时，不再无限提示重启。
3. 在保留旧密文的前提下，提供 Awaken 内的诊断和恢复引导；任何系统重新认证都必须由操作系统 UI 或用户交互完成。
4. 在安全写入成功前，禁止 UI 将更新后的 key 标记为已保存。
5. 固化 app name、bundle ID、签名 identity、Windows application ID 与 `userData` 路径，并持续验证历史 namespace 迁移。
6. 记录不含秘密的 `credential_decrypted`、`transport_started`、`provider_response_received`、恢复动作与写入尝试，方便区分本地解密、网络传输和供应商错误。
7. 评估 macOS Data Protection Keychain 作为长期方向。Apple DTS 建议把它作为可能避免同类登录钥匙串变化的方向评估，但没有给出适用于 Awaken 的保证；迁移必须单独设计兼容、回滚与数据保留方案。
8. Windows 继续基于 DPAPI 实现相同产品语义，单独覆盖密码正常修改、管理员重置、跨用户/跨机器与 `userData` 丢失场景。
9. 统一可信内存边界契约：明确明文 key 在 Electron 主进程及 `awaken-core` thread/Agent 缓存中的实际存活范围，设计 profile 变更、thread 淘汰和进程退出时的清理策略，并验证不持久化、不进入 Renderer 与日志。

## 上游参考

- [Apple：errSecAuthFailed](https://developer.apple.com/documentation/security/errsecauthfailed)
- [Apple Developer Forums：macOS 26.4 后认证读取失败，锁定/解锁后恢复的同症状报告](https://developer.apple.com/forums/thread/822120)
- [Electron：safeStorage](https://www.electronjs.org/docs/latest/api/safe-storage)
- [Electron 42.4.1：safeStorage 实现](https://github.com/electron/electron/blob/v42.4.1/shell/browser/api/electron_api_safe_storage.cc)
- [Chromium：macOS Keychain provider](https://chromium.googlesource.com/chromium/src/+/148.0.7778.265/components/os_crypt/async/browser/keychain_key_provider.mm)

## 维护要求

- 每次 macOS、Electron、安全存储实现、应用 identity 或凭据 envelope 变更后，更新相关测试项与结果。
- 若后续获得 Apple Bug ID、正式根因或稳定复现步骤，在“证据与根因边界”中追加，不覆盖本次原始事实。
- 如命令、配置 schema 或正式 Desktop 生命周期入口变化，以根 `README.md` 与当前代码为准同步修订。
- 任何修订仍需遵守：不泄露秘密、不删除旧密文、不把平台命令变成普通用户流程。
