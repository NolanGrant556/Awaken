import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const args = process.argv.slice(2);
const catalogPath = args.find((arg) => !arg.startsWith("--")) ?? path.join(root, "src/managedenv/assets/catalog.json");
const catalog = JSON.parse(fs.readFileSync(catalogPath, "utf8"));
const targets = ["darwin-arm64", "darwin-x64", "windows-x64"];
const errors = [];
const unavailable = [];
const appResources = JSON.parse(fs.readFileSync(path.join(root, "src/managedenv/assets/app-resources.json"), "utf8"));
const resourceIds = new Set();
for (const resource of appResources.resources ?? []) {
  const resolver = path.resolve(root, resource.resolver ?? "");
  if (!resource.id || resourceIds.has(resource.id) || resource.source !== "app-bundled" || !resource.owner) errors.push("invalid app resource inventory entry");
  if (!resolver.startsWith(root + path.sep) || !fs.existsSync(resolver)) errors.push(`${resource.id}: app resource resolver is missing or outside checkout`);
  resourceIds.add(resource.id);
}
if (catalog.schema_version !== 1 || !catalog.revision) errors.push("invalid catalog schema/revision");
for (const [id, component] of Object.entries(catalog.components)) {
  if (!component.license || !component.source?.startsWith("https://")) errors.push(`${id}: missing source or license`);
  for (const target of targets) {
    const asset = component.assets[target];
    if (!asset) { unavailable.push(`${id}/${target}`); continue; }
    if (!component.version || !/^[a-f0-9]{64}$/.test(asset.sha256)) errors.push(`${id}/${target}: missing exact version/hash`);
    const url = new URL(asset.url);
    if (url.protocol !== "https:" || url.username || url.password || url.search || /\/latest\//.test(url.pathname)) errors.push(`${id}/${target}: mutable or credential-bearing source`);
    if (!["tar.gz", "tar.xz", "zip", "binary"].includes(asset.format)) errors.push(`${id}/${target}: unknown format`);
    if (!Object.keys(asset.executables ?? {}).length && !Object.keys(asset.resources ?? {}).length) errors.push(`${id}/${target}: no executable verification entry`);
    for (const relative of [...Object.values(asset.executables ?? {}), ...Object.values(asset.resources ?? {}), asset.strip_prefix ?? ""]) {
      if (path.posix.isAbsolute(relative) || relative.includes("\\") || relative.split("/").includes("..") || /^[a-z]:/i.test(relative)) errors.push(`${id}/${target}: escaping resource path`);
    }
  }
}
for (const [id, spec] of Object.entries(catalog.capabilities)) {
  for (const component of spec.components ?? []) if (!catalog.components[component]) errors.push(`${id}: unknown component ${component}`);
  for (const versions of [spec.python, spec.node]) for (const version of Object.values(versions ?? {})) if (!/^\d+\.\d+\.\d+$/.test(version)) errors.push(`${id}: baseline language packages must be exact versions`);
}
const office = catalog.office_bundle;
const officeUnavailable = [];
if (office) {
  if (!/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/.test(office.version ?? "") || office.layout_version !== 1 || office.spec?.office_bundle) errors.push("office: invalid bundle identity/layout");
  if (office.minimum_core_protocol !== 1) errors.push("office: unsupported minimum Core protocol");
  for (const platform of ["darwin", "windows"]) if (!/^\d+\.\d+(\.\d+)?$/.test(office.minimum_os?.[platform] ?? "")) errors.push(`office: missing minimum OS for ${platform}`);
  const spec = office.spec ?? {};
  for (const capability of ["python-office", "node-office", "pdf_max", "word.visual_qa", "excel.recalc"]) {
    const required = catalog.capabilities[capability];
    if (!required) { errors.push(`office: missing baseline ${capability}`); continue; }
    for (const id of required.components ?? []) {
      if (id !== "uv" && !spec.components?.includes(id)) errors.push(`office: ${capability} lacks ${id}`);
    }
    for (const language of ["python", "node"]) for (const [name, version] of Object.entries(required[language] ?? {})) {
      if (spec[language]?.[name] !== version) errors.push(`office: ${capability} lacks locked ${language}/${name}`);
    }
  }
  for (const id of spec.components ?? []) {
    if (!catalog.components[id] || spec.component_versions?.[id] !== catalog.components[id].version) errors.push(`office: component version differs from product lock: ${id}`);
  }
  for (const target of targets) {
    const asset = office.assets?.[target];
    if (!asset) { officeUnavailable.push(target); continue; }
    if (!/^[a-f0-9]{64}$/.test(asset.sha256 ?? "")) errors.push(`office/${target}: missing SHA-256`);
    try {
      const url = new URL(asset.url);
      if (url.protocol !== "https:" || url.username || url.password || url.search || /\/latest\//.test(url.pathname)) throw new Error();
    } catch { errors.push(`office/${target}: invalid immutable source URL`); }
    if (!["tar.gz", "zip"].includes(asset.format)) errors.push(`office/${target}: unsupported fixed-bundle format`);
    if (!Number.isSafeInteger(asset.max_bytes) || asset.max_bytes <= 0 || !Number.isSafeInteger(asset.max_expanded_bytes) || asset.max_expanded_bytes <= 0 || asset.max_expanded_bytes > 8 * 1024 ** 3) errors.push(`office/${target}: missing or invalid size budget`);
    for (const name of ["python", "node", "soffice", "pdftoppm", "pdfinfo", "pdftotext", "pdffonts", "pdfimages", "pdfseparate", "pdfunite", "pdftocairo", "pdftohtml", "pdftops", "pdfattach", "pdfdetach", "typst"]) {
      if (!asset.executables?.[name]) errors.push(`office/${target}: missing executable ${name}`);
    }
    for (const name of ["python_packages", "node_packages", "fonts", "override", "fallback", "licenses"]) {
      if (!asset.resources?.[name]) errors.push(`office/${target}: missing resource ${name}`);
    }
    for (const relative of [...Object.values(asset.executables ?? {}), ...Object.values(asset.resources ?? {})]) {
      if (typeof relative !== "string" || !relative || path.posix.isAbsolute(relative) || relative.includes("\\") || relative.includes(":") || relative.split("/").includes("..")) errors.push(`office/${target}: invalid bundle-relative path`);
    }
  }
} else {
  officeUnavailable.push(...targets);
}
if (args.includes("--office-release")) {
  for (const target of officeUnavailable) errors.push(`office/${target}: fixed office bundle is unavailable`);
}
if (args.includes("--release")) {
  for (const entry of unavailable) {
    const id = entry.split("/")[0];
    if (!catalog.components[id].optional) errors.push(`${entry}: required automatic-install asset is unavailable`);
  }
}
console.log(JSON.stringify({ catalog: catalogPath, targets, components: Object.keys(catalog.components).length, capabilities: Object.keys(catalog.capabilities).length, unavailable, officeUnavailable, errors }, null, 2));
if (errors.length) process.exitCode = 1;
