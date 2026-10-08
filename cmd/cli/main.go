package main

//  go run ./cmd/cli <файл.scala>

import (
	"fmt"
	"os"
	"strings"

	"gilblab/gilb"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: gilb-cli <путь к .scala файлу>")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("Не удалось прочитать файл:", err)
		os.Exit(1)
	}
	r := gilb.Analyze(string(data))

	fmt.Printf("Файл: %s\n%s\n", os.Args[1], strings.Repeat("=", 64))
	if r.SyntaxErrors > 0 {
		fmt.Println("обнаружены синтаксические ошибки при разборе")
	}

	fmt.Println("\n  УПРАВЛЯЮЩИЕ КОНСТРУКЦИИ")
	fmt.Println("  Строка  Конструкция  Вклад  Уровень  Текст")
	for _, b := range r.Branches {
		fmt.Printf("  %-7d %-12s +%-5d %-8d %s\n", b.Line, b.Kind, b.Contribution, b.Level, b.Text)
	}

	fmt.Println("\n  ОПЕРАТОРЫ (для N_ops)")
	for _, e := range r.Operators {
		fmt.Printf("  %-20s %d\n", e.Name, e.Count)
	}

	fmt.Println("\n  " + strings.Repeat("-", 60))
	fmt.Printf("  CL   (абсолютная сложность)      = %d\n", r.CL)
	fmt.Printf("  N_ops (число операторов)         = %d\n", r.NOps)
	fmt.Printf("  cl = CL / N_ops (относительная)  = %.4f\n", r.Cl)
	fmt.Printf("  CLI  (макс. вложенность)         = %d\n", r.CLI)
}
