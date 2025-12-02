package ui

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/platform"
)

// WizardState хранит состояние мастера конфигурации
type WizardState struct {
	Controller *core.AppController
	Window     fyne.Window

	// Tab 1: VLESS Sources
	VLESSURLEntry        *widget.Entry
	URLStatusLabel       *widget.Label
	ParserConfigEntry    *widget.Entry
	OutboundsPreview     *widget.Entry
	OutboundsPreviewText string // Храним текст для read-only режима
	CheckURLButton       *widget.Button
	ParseButton          *widget.Button

	// Parsed data
	ParserConfig       *core.ParserConfig
	GeneratedOutbounds []string
}

// ShowConfigWizard открывает окно мастера конфигурации
func ShowConfigWizard(parent fyne.Window, controller *core.AppController) {
	state := &WizardState{
		Controller: controller,
	}

	// Создаем новое окно для мастера
	wizardWindow := controller.Application.NewWindow("Config Wizard")
	wizardWindow.Resize(fyne.NewSize(920, 720))
	wizardWindow.CenterOnScreen()
	state.Window = wizardWindow

	// Создаем первую вкладку
	tab1 := createVLESSSourceTab(state)

	// Загружаем данные из существующего конфига
	if err := loadConfigFromFile(state); err != nil {
		log.Printf("ConfigWizard: Failed to load config: %v", err)
		// Показываем ошибку, но продолжаем работу с дефолтными значениями
		dialog.ShowError(fmt.Errorf("Failed to load existing config: %w", err), wizardWindow)
	}

	// Создаем контейнер с вкладками (пока только одна)
	tabs := container.NewAppTabs(
		container.NewTabItem("VLESS Sources & ParserConfig", tab1),
	)

	// Кнопки навигации (пока только Close, позже добавим Next)
	closeButton := widget.NewButton("Close", func() {
		wizardWindow.Close()
	})
	closeButton.Importance = widget.HighImportance

	buttonsContainer := container.NewHBox(
		widget.NewLabel(""), // Spacer
		closeButton,
	)

	content := container.NewBorder(
		nil,              // top
		buttonsContainer, // bottom
		nil,              // left
		nil,              // right
		tabs,             // center
	)

	wizardWindow.SetContent(content)
	wizardWindow.Show()
}

// createVLESSSourceTab создает первую вкладку с полями для VLESS URL и ParserConfig
func createVLESSSourceTab(state *WizardState) fyne.CanvasObject {
	// Секция 1: VLESS Subscription URL
	urlLabel := widget.NewLabel("VLESS Subscription URL:")
	urlLabel.Importance = widget.MediumImportance

	state.VLESSURLEntry = widget.NewEntry()
	state.VLESSURLEntry.SetPlaceHolder("https://example.com/subscription")
	state.VLESSURLEntry.Wrapping = fyne.TextWrapOff

	state.CheckURLButton = widget.NewButton("Check URL", func() {
		go checkURL(state)
	})

	state.URLStatusLabel = widget.NewLabel("")
	state.URLStatusLabel.Wrapping = fyne.TextWrapWord

	urlContainer := container.NewVBox(
		urlLabel,
		container.NewBorder(
			nil,                  // top
			nil,                  // bottom
			nil,                  // left
			state.CheckURLButton, // right - кнопка справа
			state.VLESSURLEntry,  // center - поле ввода занимает всё доступное пространство
		),
		state.URLStatusLabel,
	)

	// Секция 2: ParserConfig
	state.ParserConfigEntry = widget.NewMultiLineEntry()
	state.ParserConfigEntry.SetPlaceHolder("Enter ParserConfig JSON here...")
	state.ParserConfigEntry.Wrapping = fyne.TextWrapOff
	// Всегда начинаем с шаблона, чтобы поле не оставалось пустым при отсутствии конфигурации
	state.ParserConfigEntry.SetText(defaultParserConfigTemplate)

	// Создаем фиктивный Rectangle для установки высоты через container.NewMax
	parserHeightRect := canvas.NewRectangle(color.Transparent)
	parserHeightRect.SetMinSize(fyne.NewSize(0, 200)) // ~10 строк

	// Обертываем в Max контейнер с Rectangle для фиксации высоты
	parserConfigWithHeight := container.NewMax(
		parserHeightRect,
		state.ParserConfigEntry,
	)

	// Кнопка документации
	docButton := widget.NewButton("📖 Documentation", func() {
		docURL := "https://github.com/Leadaxe/singbox-launcher/blob/main/README.md#configuring-configjson"
		if err := platform.OpenURL(docURL); err != nil {
			dialog.ShowError(fmt.Errorf("failed to open documentation: %w", err), state.Window)
		}
	})

	parserLabel := widget.NewLabel("ParserConfig:")
	parserLabel.Importance = widget.MediumImportance

	// Кнопка Parse (располагается слева от ParserConfig)
	state.ParseButton = widget.NewButton("Parse", func() {
		go parseAndPreview(state)
	})
	state.ParseButton.Importance = widget.MediumImportance

	headerRow := container.NewHBox(
		parserLabel,
		widget.NewLabel("  "), // небольшой отступ между текстом и кнопкой
		state.ParseButton,
		layout.NewSpacer(),
		docButton,
	)

	parserContainer := container.NewVBox(
		headerRow,
		parserConfigWithHeight,
	)

	// Секция 3: Preview Generated Outbounds
	previewLabel := widget.NewLabel("Preview")
	previewLabel.Importance = widget.MediumImportance

	// Используем Entry без Disable для черного текста, но делаем его read-only через OnChanged
	state.OutboundsPreview = widget.NewMultiLineEntry()
	state.OutboundsPreview.SetPlaceHolder("Generated outbounds will appear here after clicking Parse...")
	state.OutboundsPreview.Wrapping = fyne.TextWrapOff
	state.OutboundsPreviewText = "Generated outbounds will appear here after clicking Parse..."
	state.OutboundsPreview.SetText(state.OutboundsPreviewText)
	// Делаем поле read-only, но текст остается черным (не disabled)
	state.OutboundsPreview.OnChanged = func(text string) {
		// Восстанавливаем сохраненный текст при попытке редактирования
		if text != state.OutboundsPreviewText {
			state.OutboundsPreview.SetText(state.OutboundsPreviewText)
		}
	}

	// Создаем фиктивный Rectangle для установки высоты через container.NewMax
	previewHeightRect := canvas.NewRectangle(color.Transparent)
	previewHeightRect.SetMinSize(fyne.NewSize(0, 200)) // ~10 строк

	// Обертываем в Max контейнер с Rectangle для фиксации высоты
	previewWithHeight := container.NewMax(
		previewHeightRect,
		state.OutboundsPreview,
	)

	previewContainer := container.NewVBox(
		previewLabel,
		previewWithHeight,
	)

	// Объединяем все секции
	content := container.NewVBox(
		widget.NewSeparator(),
		urlContainer,
		widget.NewSeparator(),
		parserContainer,
		widget.NewSeparator(),
		previewContainer,
		widget.NewSeparator(),
	)

	// Добавляем скролл для длинного контента
	scrollContainer := container.NewScroll(content)
	scrollContainer.SetMinSize(fyne.NewSize(900, 680))

	return scrollContainer
}

// loadConfigFromFile загружает данные из существующего config.json
func loadConfigFromFile(state *WizardState) error {
	// Проверяем наличие config.json
	if _, err := os.Stat(state.Controller.ConfigPath); os.IsNotExist(err) {
		// Конфиг не существует - оставляем значения по умолчанию
		log.Println("ConfigWizard: config.json not found, using default values")
		return nil
	}

	// Извлекаем ParserConfig
	parserConfig, err := core.ExtractParcerConfig(state.Controller.ConfigPath)
	if err != nil {
		// Если не удалось извлечь - оставляем значения по умолчанию
		log.Printf("ConfigWizard: Failed to extract ParserConfig: %v", err)
		return nil // Не критическая ошибка
	}

	state.ParserConfig = parserConfig

	// Заполняем поле URL
	if len(parserConfig.ParserConfig.Proxies) > 0 {
		state.VLESSURLEntry.SetText(parserConfig.ParserConfig.Proxies[0].Source)
	}

	parserConfigJSON, err := serializeParserConfig(parserConfig)
	if err != nil {
		log.Printf("ConfigWizard: Failed to serialize ParserConfig: %v", err)
		return err
	}

	state.ParserConfigEntry.SetText(string(parserConfigJSON))

	log.Println("ConfigWizard: Successfully loaded config from file")
	return nil
}

// checkURL проверяет доступность URL подписки
func checkURL(state *WizardState) {
	url := strings.TrimSpace(state.VLESSURLEntry.Text)
	if url == "" {
		fyne.Do(func() {
			state.URLStatusLabel.SetText("❌ Please enter a URL")
			state.URLStatusLabel.Importance = widget.DangerImportance
		})
		return
	}

	// Обновляем UI
	fyne.Do(func() {
		state.URLStatusLabel.SetText("⏳ Checking...")
		state.URLStatusLabel.Importance = widget.MediumImportance
		state.CheckURLButton.Disable()
	})

	// Проверяем URL в горутине
	content, err := core.FetchSubscription(url)
	if err != nil {
		fyne.Do(func() {
			state.URLStatusLabel.SetText(fmt.Sprintf("❌ Failed: %v", err))
			state.URLStatusLabel.Importance = widget.DangerImportance
			state.CheckURLButton.Enable()
		})
		return
	}

	// Проверяем, что контент не пустой и содержит хотя бы одну строку
	lines := strings.Split(string(content), "\n")
	validLines := 0
	previewLines := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && (strings.HasPrefix(line, "vless://") || strings.HasPrefix(line, "vmess://") || strings.HasPrefix(line, "trojan://") || strings.HasPrefix(line, "ss://")) {
			validLines++
			previewLines = append(previewLines, fmt.Sprintf("%d. %s", validLines, line))
		}
	}

	if validLines == 0 {
		fyne.Do(func() {
			state.URLStatusLabel.SetText("❌ URL is accessible but contains no valid proxy links")
			state.URLStatusLabel.Importance = widget.DangerImportance
			state.CheckURLButton.Enable()
		})
		return
	}

	fyne.Do(func() {
		state.URLStatusLabel.SetText(fmt.Sprintf("✅ Working! Found %d valid proxy link(s)", validLines))
		state.URLStatusLabel.Importance = widget.SuccessImportance
		state.CheckURLButton.Enable()
		if len(previewLines) > 0 {
			setPreviewText(state, strings.Join(previewLines, "\n"))
		} else {
			setPreviewText(state, "No valid proxy links found to preview.")
		}
	})
}

// parseAndPreview парсит ParserConfig и генерирует предпросмотр outbounds
func parseAndPreview(state *WizardState) {
	fyne.Do(func() {
		state.ParseButton.Disable()
		state.ParseButton.SetText("Parsing...")
		setPreviewText(state, "Parsing configuration...")
	})

	// Парсим ParserConfig из поля
	parserConfigJSON := strings.TrimSpace(state.ParserConfigEntry.Text)
	if parserConfigJSON == "" {
		fyne.Do(func() {
			setPreviewText(state, "Error: ParserConfig is empty")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
		})
		return
	}

	var parserConfig core.ParserConfig
	if err := json.Unmarshal([]byte(parserConfigJSON), &parserConfig); err != nil {
		fyne.Do(func() {
			setPreviewText(state, fmt.Sprintf("Error: Failed to parse ParserConfig JSON: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
		})
		return
	}

	// Проверяем наличие URL
	url := strings.TrimSpace(state.VLESSURLEntry.Text)
	if url == "" {
		fyne.Do(func() {
			setPreviewText(state, "Error: VLESS URL is empty")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
		})
		return
	}

	// Обновляем URL в конфиге, если он отличается
	if len(parserConfig.ParserConfig.Proxies) > 0 {
		parserConfig.ParserConfig.Proxies[0].Source = url
	} else {
		// Добавляем новый источник, если его нет
		parserConfig.ParserConfig.Proxies = []core.ProxySource{
			{Source: url},
		}
	}

	// Устанавливаем значение по умолчанию (из @ParcerConfig, если доступен)
	state.ParserConfigEntry.SetText(defaultParserConfigTemplate)
	// Загружаем подписку
	fyne.Do(func() {
		setPreviewText(state, "Downloading subscription...")
	})

	content, err := core.FetchSubscription(url)
	if err != nil {
		fyne.Do(func() {
			setPreviewText(state, fmt.Sprintf("Error: Failed to fetch subscription: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
		})
		return
	}

	// Парсим узлы из подписки
	fyne.Do(func() {
		setPreviewText(state, "Parsing nodes from subscription...")
	})

	allNodes := make([]*core.ParsedNode, 0)
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var skipFilters []map[string]string
		if len(parserConfig.ParserConfig.Proxies) > 0 {
			skipFilters = parserConfig.ParserConfig.Proxies[0].Skip
		}

		node, err := parseNodeFromString(line, skipFilters)
		if err != nil {
			log.Printf("ConfigWizard: Failed to parse node: %v", err)
			continue
		}

		if node != nil {
			allNodes = append(allNodes, node)
		}
	}

	if len(allNodes) == 0 {
		fyne.Do(func() {
			setPreviewText(state, "Error: No valid nodes found in subscription")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
		})
		return
	}

	// Генерируем JSON для узлов
	fyne.Do(func() {
		setPreviewText(state, "Generating outbounds...")
	})

	selectorsJSON := make([]string, 0)

	// Генерируем JSON для всех узлов
	for _, node := range allNodes {
		nodeJSON, err := generateNodeJSONForPreview(node)
		if err != nil {
			log.Printf("ConfigWizard: Failed to generate JSON for node: %v", err)
			continue
		}
		selectorsJSON = append(selectorsJSON, nodeJSON)
	}

	// Генерируем селекторы
	for _, outboundConfig := range parserConfig.ParserConfig.Outbounds {
		selectorJSON, err := generateSelectorForPreview(allNodes, outboundConfig)
		if err != nil {
			log.Printf("ConfigWizard: Failed to generate selector: %v", err)
			continue
		}
		if selectorJSON != "" {
			selectorsJSON = append(selectorsJSON, selectorJSON)
		}
	}

	// Формируем итоговый текст для предпросмотра
	previewText := strings.Join(selectorsJSON, "\n")

	fyne.Do(func() {
		setPreviewText(state, previewText)
		state.ParseButton.Enable()
		state.ParseButton.SetText("Parse")
		state.GeneratedOutbounds = selectorsJSON
		state.ParserConfig = &parserConfig
	})
}

func setPreviewText(state *WizardState, text string) {
	state.OutboundsPreviewText = text
	if state.OutboundsPreview != nil {
		state.OutboundsPreview.SetText(text)
	}
}

// parseNodeFromString парсит узел из строки (обертка над core.ParseNode)
func parseNodeFromString(uri string, skipFilters []map[string]string) (*core.ParsedNode, error) {
	return core.ParseNode(uri, skipFilters)
}

// generateNodeJSONForPreview генерирует JSON для узла (обертка над core.GenerateNodeJSON)
func generateNodeJSONForPreview(node *core.ParsedNode) (string, error) {
	return core.GenerateNodeJSON(node)
}

// generateSelectorForPreview генерирует JSON для селектора (обертка над core.GenerateSelector)
func generateSelectorForPreview(allNodes []*core.ParsedNode, outboundConfig core.OutboundConfig) (string, error) {
	return core.GenerateSelector(allNodes, outboundConfig)
}

const defaultParserConfigTemplate = `{
  "version": 1,
  "ParserConfig": {
    "proxies": [{ "source": "https://USE_YOUR_SUBSCRIPTION_URL_HERE" }],
    "outbounds": [
      {
        "tag": "proxy-out",
        "type": "selector",
        "options": { "interrupt_exist_connections": true },
        "outbounds": {
          "proxies": { "tag": "!/(DO_NOT_USE_THIS)/i" },
          "addOutbounds": ["direct-out"]
        },
        "comment": "Proxy group for all connections"
      }
    ]
  }
}`

func serializeParserConfig(parserConfig *core.ParserConfig) (string, error) {
	if parserConfig == nil {
		return "", fmt.Errorf("parserConfig is nil")
	}
	configToSerialize := map[string]interface{}{
		"version": parserConfig.Version,
		"ParserConfig": map[string]interface{}{
			"proxies":   parserConfig.ParserConfig.Proxies,
			"outbounds": parserConfig.ParserConfig.Outbounds,
		},
	}
	data, err := json.MarshalIndent(configToSerialize, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
