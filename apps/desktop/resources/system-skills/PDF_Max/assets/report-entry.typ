// 原生专业报告入口。prepare_document.py 会在项目内写入结构化配置。

#import "report-theme.typ": awaken-report-theme, report-accent, report-muted

#let config = json(".typst-config.json")
#let meta-title = config.title
#let meta-subtitle = config.subtitle
#let meta-author = config.author

#show: awaken-report-theme.with(
  title: meta-title,
  author: meta-author,
  language: config.language,
  region: config.region,
  body-fonts: config.body_fonts,
  heading-fonts: config.heading_fonts,
  first-line-indent: 2em,
  running-header: true,
)

#page(margin: (top: 28%, x: 2.2cm), numbering: none, header: none)[
  #set par(first-line-indent: 0em, justify: false)
  #align(center)[
    #text(size: 27pt, weight: "bold", fill: report-accent)[#meta-title]
    #if meta-subtitle != "" [
      #v(0.65em)
      #text(size: 13pt, fill: report-muted)[#meta-subtitle]
    ]
    #v(2.2em)
    #line(length: 34%, stroke: 0.6pt + rgb("#98a2b3"))
    #v(1.8em)
    #if meta-author != "" [
      #text(size: 11pt)[#config.author_label：#meta-author]
      #linebreak()
    ]
    #text(size: 11pt)[#config.date_label：#datetime.today().display("[year]-[month]-[day]")]
  ]
]

#page(numbering: none, header: none)[
  #set par(first-line-indent: 0em)
  #outline(title: config.outline_title, indent: 1.5em)
]

#counter(page).update(1)

#heading(level: 1, config.intro_heading)

#config.body_placeholder
