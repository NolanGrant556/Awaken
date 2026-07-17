// Awaken Typst PDF Skill 的中性专业报告主题。
// 字体、语言和文档元数据均由 prepare_document.py 明确注入。

#let report-accent = rgb("#2457a6")
#let report-ink = rgb("#1f2937")
#let report-muted = rgb("#667085")

#let awaken-report-theme(
  title: none,
  author: none,
  language: "zh",
  region: "cn",
  body-fonts: ("Libertinus Serif",),
  heading-fonts: ("Libertinus Serif",),
  first-line-indent: 2em,
  running-header: true,
  body,
) = {
  if title != none or author != none {
    set document(title: title, author: author)
  }

  set page(
    paper: "a4",
    margin: (top: 2.45cm, bottom: 2.35cm, x: 2.15cm),
    numbering: "1",
    number-align: center,
    header: if running-header {
      context {
        if counter(page).get().first() > 1 {
          set text(font: body-fonts, size: 8.5pt, fill: report-muted)
          grid(
            columns: (1fr, auto),
            column-gutter: 1em,
            align(left, title),
            align(right, author),
          )
          v(-0.55em)
          line(length: 100%, stroke: 0.45pt + rgb("#d0d5dd"))
        }
      }
    } else {
      none
    },
  )

  set text(
    font: body-fonts,
    size: 10.8pt,
    fill: report-ink,
    lang: language,
    region: region,
  )
  set par(
    justify: true,
    leading: 0.72em,
    spacing: 0.76em,
    first-line-indent: (amount: first-line-indent, all: true),
  )

  set heading(numbering: "1.1")
  show heading: set text(font: heading-fonts, fill: report-accent)
  show heading.where(level: 1): it => block(
    above: 1.25em,
    below: 0.72em,
    breakable: false,
  )[
    #set text(size: 17pt, weight: "bold")
    #it
  ]
  show heading.where(level: 2): it => block(
    above: 1em,
    below: 0.55em,
    breakable: false,
  )[
    #set text(size: 13pt, weight: "semibold")
    #it
  ]
  show raw.where(block: true): it => block(
    width: 100%,
    fill: rgb("#f4f6f8"),
    inset: 10pt,
    radius: 4pt,
    breakable: true,
  )[
    #set text(size: 9pt)
    #it
  ]
  show link: set text(fill: report-accent)
  show figure: set block(breakable: true)

  body
}
