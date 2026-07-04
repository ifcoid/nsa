// Package latex mengonversi Markdown manuskrip ke LaTeX dasar (scaffold submission).
package latex

import (
	"fmt"
	"strings"
)

// escape menangani karakter khusus LaTeX di teks biasa.
func escape(s string) string {
	r := strings.NewReplacer(
		"\\", "\\textbackslash{}",
		"&", "\\&", "%", "\\%", "$", "\\$", "#", "\\#",
		"_", "\\_", "{", "\\{", "}", "\\}", "~", "\\textasciitilde{}", "^", "\\textasciicircum{}",
	)
	return r.Replace(s)
}

// inline menangani **bold**, *italic*, dan `code` setelah escape.
func inline(s string) string {
	s = escape(s)
	// bold **x**
	for strings.Count(s, "**") >= 2 {
		s = strings.Replace(s, "**", "\\textbf{", 1)
		s = strings.Replace(s, "**", "}", 1)
	}
	return s
}

// MarkdownToLatex mengonversi Markdown (heading/paragraf/list/tabel sederhana) ke dokumen LaTeX lengkap.
func MarkdownToLatex(title, body string) string {
	var out strings.Builder
	out.WriteString("\\documentclass[12pt]{article}\n")
	out.WriteString("\\usepackage[utf8]{inputenc}\n\\usepackage[T1]{fontenc}\n")
	out.WriteString("\\usepackage{times}\n\\usepackage[margin=1in]{geometry}\n")
	out.WriteString("\\usepackage{setspace}\n\\usepackage{graphicx}\n\\usepackage{hyperref}\n\\usepackage{longtable}\n")
	out.WriteString("\\doublespacing\n")
	if title != "" {
		out.WriteString("\\title{" + escape(title) + "}\n")
	}
	out.WriteString("\\date{}\n\\begin{document}\n")
	if title != "" {
		out.WriteString("\\maketitle\n")
	}

	lines := strings.Split(body, "\n")
	inList := false
	inTable := false
	closeList := func() {
		if inList {
			out.WriteString("\\end{itemize}\n")
			inList = false
		}
	}
	closeTable := func() {
		if inTable {
			out.WriteString("\\end{longtable}\n")
			inTable = false
		}
	}
	inCode := false
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		// Blok kode ```...``` → verbatim (abaikan karakter khusus LaTeX seperti {,},_,& pada JSON).
		if strings.HasPrefix(t, "```") {
			closeList()
			closeTable()
			if !inCode {
				out.WriteString("\\begin{verbatim}\n")
				inCode = true
			} else {
				out.WriteString("\\end{verbatim}\n")
				inCode = false
			}
			continue
		}
		if inCode {
			out.WriteString(ln + "\n") // mentah, tanpa escape
			continue
		}
		switch {
		case t == "":
			closeList()
			closeTable()
			out.WriteString("\n")
		case strings.HasPrefix(t, "#### "):
			closeList()
			closeTable()
			out.WriteString("\\paragraph{" + inline(strings.TrimPrefix(t, "#### ")) + "}\n")
		case strings.HasPrefix(t, "### "):
			closeList()
			closeTable()
			out.WriteString("\\subsubsection*{" + inline(strings.TrimPrefix(t, "### ")) + "}\n")
		case strings.HasPrefix(t, "## "):
			closeList()
			closeTable()
			out.WriteString("\\subsection*{" + inline(strings.TrimPrefix(t, "## ")) + "}\n")
		case strings.HasPrefix(t, "# "):
			closeList()
			closeTable()
			out.WriteString("\\section*{" + inline(strings.TrimPrefix(t, "# ")) + "}\n")
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* "):
			closeTable()
			if !inList {
				out.WriteString("\\begin{itemize}\n")
				inList = true
			}
			out.WriteString("\\item " + inline(t[2:]) + "\n")
		case strings.HasPrefix(t, "|") && strings.Contains(t, "|"):
			closeList()
			cells := splitRow(t)
			if isSeparatorRow(cells) {
				continue // baris pemisah markdown table
			}
			if !inTable {
				out.WriteString("\\begin{longtable}{" + strings.Repeat("l", len(cells)) + "}\n\\hline\n")
				inTable = true
			}
			esc := make([]string, len(cells))
			for i, c := range cells {
				esc[i] = inline(strings.TrimSpace(c))
			}
			out.WriteString(strings.Join(esc, " & ") + " \\\\\n\\hline\n")
		default:
			closeList()
			closeTable()
			out.WriteString(inline(t) + "\n")
		}
	}
	closeList()
	closeTable()
	if inCode {
		out.WriteString("\\end{verbatim}\n")
	}
	out.WriteString("\n\\end{document}\n")
	return out.String()
}

func splitRow(t string) []string {
	t = strings.Trim(t, "|")
	return strings.Split(t, "|")
}

func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if strings.Trim(c, "-: ") != "" {
			return false
		}
	}
	return true
}

// Note menambahkan instruksi Pandoc opsional sebagai komentar (untuk poles).
func PandocHint() string {
	return fmt.Sprintf("%% Untuk hasil lebih rapi: pandoc manuscript_final.md -o out.tex --citeproc --bibliography=reference.bib\n")
}
