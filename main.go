package main

import (
	"fmt"
	"os"
	"os/signal"
)

func main() {
	if err := initLogger(); err != nil {
		fmt.Printf("[!] Ошибка инициализации логгера: %v\n", err)
		os.Exit(1)
	}
	defer closeLogger()

	if err := LoadConfig(); err != nil {
		logError("Ошибка загрузки конфига: %v", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		runBest()
		return
	}

	switch os.Args[1] {
	case "--find":
		runFind()
	case "--status":
		runStatus()
	case "--stop":
		runStop()
	case "--watch":
		runWatch()
	case "--server":
		runServer()
	case "--update":
		runUpdate()
	default:
		logInfo("Использование:")
		logInfo("  zapret-core           — запустить лучшую известную стратегию")
		logInfo("  zapret-core --find    — найти рабочую стратегию")
		logInfo("  zapret-core --status  — показать статус")
		logInfo("  zapret-core --stop    — остановить")
		logInfo("  zapret-core --watch   — мониторинг + автоподбор при поломке")
		logInfo("  zapret-core --server  — запустить HTTP API сервер на :7432")
		logInfo("  zapret-core --update  — обновить списки из GitHub")
	}
}

// runBest loads and runs the best known strategy for the current provider
func runBest() {
	logInfo("Определяем провайдера...")
	provider := GetProvider()
	logInfo("Провайдер: %s (%s)", provider.ASN, provider.Org)

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Ошибка загрузки knowledge: %v", err)
		os.Exit(1)
	}

	vectors := kb.BestForASN(provider.ASN, 1)
	if len(vectors) == 0 {
		logWarn("Нет известных стратегий для этого провайдера. Запустите --find")
		os.Exit(1)
	}

	strategy := VectorToStrategy(vectors[0], 0)

	logInfo("Запускаем стратегию: %s", strategy.Name)
	err = StartWinws(strategy)
	if err != nil {
		logError("Ошибка запуска: %v", err)
		os.Exit(1)
	}
	logInfo("Запущено. Ctrl+C для остановки.")
	select {} // block forever
}

// runFind iterates through strategies to find a working one
func runFind() {
	logInfo("Определяем провайдера...")
	provider := GetProvider()
	logInfo("Провайдер: %s (%s)", provider.ASN, provider.Org)

	logInfo("Проверяем конфликты...")
	conflicts := CheckConflicts()
	if len(conflicts) > 0 {
		logWarn("Обнаружены конфликты:")
		for _, c := range conflicts {
			logWarn("    - %s", c)
		}
		logError("Устраните конфликты и запустите снова.")
		os.Exit(1)
	}
	logInfo("Конфликтов нет.")

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Ошибка загрузки knowledge: %v", err)
		os.Exit(1)
	}

	opt := NewOptimizer(provider.ASN, kb)

	logInfo("Начинаем перебор стратегий...")
	result, vector := opt.Run()

	if result == nil {
		logError("Рабочая стратегия не найдена.")
		os.Exit(1)
	}

	logInfo("Найдена рабочая стратегия: %s", result.Name)
	kb.Record(provider.ASN, vector, 1.0)
	logInfo("Стратегия сохранена в knowledge.")
}

// runStatus shows the current state
func runStatus() {
	running := IsWinwsRunning()
	if running {
		logInfo("winws запущен")
	} else {
		logWarn("winws не запущен")
	}

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Ошибка загрузки knowledge: %v", err)
		return
	}

	provider := GetProvider()
	vectors := kb.BestForASN(provider.ASN, 1)
	if len(vectors) > 0 {
		strategy := VectorToStrategy(vectors[0], 0)
		logInfo("Лучшая известная стратегия для %s: %s", provider.ASN, strategy.Name)
	}
}

// runStop stops winws
func runStop() {
	logInfo("Останавливаем winws...")
	err := StopWinws()
	if err != nil {
		logError("Ошибка: %v", err)
		os.Exit(1)
	}
	logInfo("Остановлено.")
}

// runWatch starts watchdog with auto-recovery on failure
func runWatch() {
	logInfo("Запускаем watchdog...")
	provider := GetProvider()
	logInfo("Провайдер: %s (%s)", provider.ASN, provider.Org)

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Ошибка загрузки knowledge: %v", err)
		os.Exit(1)
	}

	StartWatchdog(provider.ASN, kb)
}

// runServer starts the HTTP API server
func runServer() {
	provider := GetProvider()
	logInfo("Провайдер: %s (%s)", provider.ASN, provider.Org)

	kb, err := LoadKnowledge()
	if err != nil {
		logError("Ошибка загрузки knowledge: %v", err)
		os.Exit(1)
	}

	srv := NewAPIServer(kb, provider)
	logInfo("Starting API server on 127.0.0.1:7432")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		logInfo("Shutting down server...")
		srv.Stop()
	}()

	if err := srv.Start("127.0.0.1:7432"); err != nil {
		logError("Server error: %v", err)
		os.Exit(1)
	}
}

// runUpdate updates list files from GitHub
func runUpdate() {
	logInfo("Обновление списков из GitHub...")

	err := UpdateLists(func(current, total int, filename string) {
		logInfo("[%d/%d] Обновление %s...", current, total, filename)
	})

	if err != nil {
		logError("Ошибка обновления: %v", err)
		os.Exit(1)
	}

	logInfo("Списки успешно обновлены.")
}