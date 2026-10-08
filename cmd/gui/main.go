package main

import (
	"fmt"
	"io"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"gilblab/gilb"
)

const example = `object Demo {
  def calculateBonus(age: Int, purchases: Int, weekday: String): Int = {
    var bonus = 0

    if (age < 18) {
      bonus += 5
    } else if (age < 30) {
      bonus += 3
    } else {
      bonus += 2
    }

    if (purchases > 0) {
      for (position <- 0 until purchases) {
        if (position % 2 == 0) {
          bonus += 1
          if (bonus > 10) {
            bonus -= 2
            if (bonus > 11) {
              bonus -= 2
            }
          }
        }
      }
    }

    var counter = 0
    while (counter < purchases) {
      if (counter % 3 == 0) bonus += 1
      counter += 1
    }

    do {
      counter -= 1
    } while (counter > 0)

    weekday match {
      case "monday"    => bonus += 3
      case "tuesday"   => bonus += 2
      case "wednesday" => bonus += 4
      case _           => bonus += 1
    }

    bonus
  }
}
`

func main() {
	a := app.NewWithID("gilb.lab2")
	w := a.NewWindow("LAB2 — Метрика Джилба")
	w.Resize(fyne.NewSize(1000, 700))

	editor := widget.NewMultiLineEntry()
	editor.SetPlaceHolder("Вставь код на Scala или открой файл...")
	editor.Wrapping = fyne.TextWrapOff

	btnOpen := widget.NewButtonWithIcon("Открыть файл", theme.FolderOpenIcon(), func() {
		fd := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if r == nil {
				return
			}
			defer r.Close()
			data, err := io.ReadAll(r)
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			editor.SetText(string(data))
		}, w)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".scala", ".sc", ".txt"}))
		fd.Show()
	})

	btnExample := widget.NewButtonWithIcon("Загрузить пример", theme.DocumentIcon(), func() {
		editor.SetText(example)
	})

	btnSave := widget.NewButtonWithIcon("Сохранить файл", theme.DocumentSaveIcon(), func() {
		fd := dialog.NewFileSave(func(wr fyne.URIWriteCloser, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if wr == nil {
				return
			}
			defer wr.Close()
			if _, err = wr.Write([]byte(editor.Text)); err != nil {
				dialog.ShowError(err, w)
			}
		}, w)
		fd.SetFileName("source.scala")
		fd.Show()
	})

	btnCalc := widget.NewButtonWithIcon("Анализировать", theme.MediaPlayIcon(), func() {
		if editor.Text == "" {
			dialog.ShowInformation("Пусто", "Нет кода для анализа", w)
			return
		}
		showResults(a, gilb.AnalyzeCached(editor.Text))
	})
	btnCalc.Importance = widget.HighImportance

	w.SetContent(container.NewBorder(
		container.NewHBox(btnOpen, btnExample, btnSave, widget.NewSeparator(), btnCalc),
		nil, nil, nil,
		container.NewPadded(editor),
	))
	w.ShowAndRun()
}

func showResults(a fyne.App, r *gilb.Result) {
	rw := a.NewWindow("Результаты — метрика Джилба")
	rw.Resize(fyne.NewSize(1100, 650))

	info := fmt.Sprintf("Строк: %d", r.Lines)
	if r.SyntaxErrors > 0 {
		info += "   |   ! обнаружены синтаксические ошибки при разборе"
	}

	metrics := newTable(
		[]string{"Показатель", "Значение"},
		[][]string{
			{"CL — абсолютная сложность", strconv.Itoa(r.CL)},
			{"N_ops — число операторов", strconv.Itoa(r.NOps)},
			{"cl — относительная сложность", fmt.Sprintf("%.4f", r.Cl)},
			{"CLI — макс. вложенность", strconv.Itoa(r.CLI)},
		},
		[]float32{300, 160},
	)

	opRows := make([][]string, 0, len(r.Operators))
	for _, e := range r.Operators {
		opRows = append(opRows, []string{e.Name, strconv.Itoa(e.Count)})
	}
	ops := container.NewBorder(
		widget.NewLabelWithStyle("Итого N_ops = "+strconv.Itoa(r.NOps), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		newTable([]string{"Оператор", "Количество"}, opRows, []float32{300, 120}),
	)

	brRows := make([][]string, 0, len(r.Branches))
	for _, b := range r.Branches {
		brRows = append(brRows, []string{
			strconv.Itoa(b.Line), b.Kind, b.Text, "+" + strconv.Itoa(b.Contribution), strconv.Itoa(b.Level),
		})
	}
	branches := newTable(
		[]string{"Строка", "Конструкция", "Текст", "Вклад в CL", "Уровень"},
		brRows,
		[]float32{70, 110, 560, 100, 80},
	)

	rw.SetContent(container.NewBorder(
		container.NewPadded(widget.NewLabel(info)), nil, nil, nil,
		container.NewAppTabs(
			container.NewTabItem("Метрики", metrics),
			container.NewTabItem("Операторы (N_ops)", ops),
			container.NewTabItem("Управляющие конструкции", branches),
		),
	))
	rw.Show()
}

func newTable(headers []string, rows [][]string, widths []float32) *widget.Table {
	t := widget.NewTable(
		func() (int, int) { return len(rows) + 1, len(headers) },
		func() fyne.CanvasObject {
			l := widget.NewLabel("template template")
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(id widget.TableCellID, o fyne.CanvasObject) {
			l := o.(*widget.Label)
			if id.Row == 0 {
				l.TextStyle = fyne.TextStyle{Bold: true}
				l.SetText(headers[id.Col])
				return
			}
			l.TextStyle = fyne.TextStyle{}
			l.SetText(rows[id.Row-1][id.Col])
		},
	)
	for i, wd := range widths {
		t.SetColumnWidth(i, wd)
	}
	return t
}
