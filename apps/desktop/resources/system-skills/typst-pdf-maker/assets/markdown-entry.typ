// Markdown 适配入口。外部包按固定 Universe 版本在首次编译时获取。

#import "@preview/cmarker:0.1.10"
#import "@preview/mitex:0.2.7": mitex
#import "report-theme.typ": awaken-report-theme

#let config = json(".typst-config.json")

#show: awaken-report-theme.with(
  title: none,
  author: none,
  language: config.language,
  region: config.region,
  body-fonts: config.body_fonts,
  heading-fonts: config.heading_fonts,
  first-line-indent: 0em,
  running-header: false,
)

#show table.cell.where(y: 0): set text(weight: "bold")
#show table: it => block(width: 100%, breakable: true)[#it]

#cmarker.render(
  read(config.markdown_source),
  math: mitex,
)
