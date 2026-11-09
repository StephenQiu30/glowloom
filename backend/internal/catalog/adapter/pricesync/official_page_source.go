package pricesync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/html"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

const (
	maxOfficialPageBytes  = 8 << 20
	officialPageParserV1  = "official_page_review_v1"
	deepSeekTimedParserV1 = "deepseek_cn_2026_timed_token_v1"
)

var officialPricePages = map[string]string{
	"volcengine_ark": "https://docs.volcengine.com/docs/ark/model-pricing",
	"deepseek":       "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/",
	"openai":         "https://developers.openai.com/api/docs/pricing",
}

// OfficialPricePageSource snapshots bounded excerpts from first-party pricing pages.
// It does not interpret human-readable rows as executable price rules.
type OfficialPricePageSource struct{ client *http.Client }

// NewOfficialPricePageSource constructs a client restricted to official pricing hosts.
func NewOfficialPricePageSource(client *http.Client) *OfficialPricePageSource {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	} else {
		clone := *client
		client = &clone
	}
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" || !officialPriceHost(request.URL.Host) {
			return errors.New("official pricing page redirected outside its allowlist")
		}
		if len(via) >= 3 {
			return errors.New("too many official pricing page redirects")
		}
		if previousRedirect != nil {
			return previousRedirect(request, via)
		}
		return nil
	}
	return &OfficialPricePageSource{client: client}
}

// FetchOfficialPageCandidates snapshots each configured first-party page once and emits review-only candidates.
func (s *OfficialPricePageSource) FetchOfficialPageCandidates(ctx context.Context, targets []application.OfficialPricePageTarget, fetchedAt time.Time) ([]domain.PriceCandidate, error) {
	if s == nil || s.client == nil || fetchedAt.IsZero() || len(targets) > 1000 {
		return nil, ErrInvalidOpenRouterPricing
	}
	groups := make(map[string][]application.OfficialPricePageTarget)
	urls := make([]string, 0)
	for _, target := range targets {
		expected, ok := officialPricePages[target.AdapterKey]
		if !ok || target.SourceURL != expected || !validPageTarget(target) {
			return nil, ErrInvalidOpenRouterPricing
		}
		if _, exists := groups[target.SourceURL]; !exists {
			urls = append(urls, target.SourceURL)
		}
		groups[target.SourceURL] = append(groups[target.SourceURL], target)
	}
	candidates := make([]domain.PriceCandidate, 0, len(targets))
	var sourceErrors []error
	for _, sourceURL := range urls {
		body, err := s.fetch(ctx, sourceURL)
		if err != nil {
			sourceErrors = append(sourceErrors, fmt.Errorf("fetch official price page %q: %w", sourceURL, err))
			continue
		}
		tables := extractHTMLTables(body)
		for _, target := range groups[sourceURL] {
			rows := matchingPriceRows(tables, target.ProviderModelID)
			snapshot := map[string]any{
				"evidence_kind":            "official_page_excerpt",
				"match_status":             "matched",
				"matched_rows":             rows,
				"previous_reference_lines": target.ReferenceLines,
			}
			if len(rows) == 0 {
				snapshot["match_status"] = "no_model_row"
				pageHash := sha256.Sum256(body)
				snapshot["page_hash"] = hex.EncodeToString(pageHash[:])
			}
			raw, err := json.Marshal(snapshot)
			if err != nil {
				return nil, fmt.Errorf("encode official price excerpt: %w", err)
			}
			candidate, err := officialPageCandidate(target, rows, raw, fetchedAt)
			if err != nil {
				return nil, fmt.Errorf("build official price candidate for %q: %w", target.ProviderModelID, err)
			}
			candidates = append(candidates, candidate)
		}
	}
	return candidates, errors.Join(sourceErrors...)
}

func (s *OfficialPricePageSource) fetch(ctx context.Context, sourceURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build page request: %w", err)
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml,text/markdown;q=0.9,*/*;q=0.8")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		if closeErr := response.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("close unsuccessful page response: %w", closeErr)
		}
		return nil, fmt.Errorf("page returned HTTP %d", response.StatusCode)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxOfficialPageBytes+1))
	closeErr := response.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read page: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close page response: %w", closeErr)
	}
	if len(body) > maxOfficialPageBytes {
		return nil, ErrInvalidOpenRouterPricing
	}
	return body, nil
}

func validPageTarget(target application.OfficialPricePageTarget) bool {
	if strings.TrimSpace(target.ProviderModelID) == "" || len(target.ProviderModelID) > 256 ||
		(target.Currency != "CNY" && target.Currency != "USD") || len(target.ReferenceLines) > 20 {
		return false
	}
	for _, line := range target.ReferenceLines {
		if len(line) > 2000 || strings.ContainsRune(line, 0) {
			return false
		}
	}
	if target.Unit == nil {
		return true
	}
	switch *target.Unit {
	case domain.PricePerImage, domain.PricePerSecond, domain.PricePerRequest, domain.PricePer1KTokens, domain.PricePer1KChars:
		return true
	default:
		return false
	}
}

func officialPageCandidate(target application.OfficialPricePageTarget, rows []string, snapshot []byte, fetchedAt time.Time) (domain.PriceCandidate, error) {
	if rule, ok := deepSeekTimedTokenRule(target, rows, fetchedAt); ok {
		unit := domain.PricePer1KTokens
		hash := sha256.Sum256(snapshot)
		hexHash := hex.EncodeToString(hash[:])
		candidate := domain.PriceCandidate{
			ID:         uuid.NewSHA1(uuid.NameSpaceURL, []byte("glowloom:deepseek-time-price:"+target.ProviderModelID+":"+hexHash)),
			AdapterKey: target.AdapterKey, ProviderModelID: target.ProviderModelID, SourceURL: target.SourceURL,
			SourceHash: hexHash, ParserVersion: deepSeekTimedParserV1, SourceFetchedAt: fetchedAt.UTC(),
			Currency: target.Currency, Unit: &unit, Rule: rule, SourcePricing: snapshot,
			Status: domain.PriceCandidatePendingReview,
		}
		if err := candidate.Validate(); err != nil {
			return domain.PriceCandidate{}, err
		}
		return candidate, nil
	}
	if rule, ok := seedreamFlashRule(target, rows); ok {
		unit := domain.PricePerImage
		hash := sha256.Sum256(snapshot)
		hexHash := hex.EncodeToString(hash[:])
		candidate := domain.PriceCandidate{
			ID:         uuid.NewSHA1(uuid.NameSpaceURL, []byte("glowloom:seedream-flash:"+target.ProviderModelID+":"+hexHash)),
			AdapterKey: target.AdapterKey, ProviderModelID: target.ProviderModelID, SourceURL: target.SourceURL,
			SourceHash: hexHash, ParserVersion: "volcengine_seedream_flash_image_v1", SourceFetchedAt: fetchedAt.UTC(),
			Currency: target.Currency, Unit: &unit, Rule: rule, SourcePricing: snapshot,
			Status: domain.PriceCandidatePendingReview,
		}
		if err := candidate.Validate(); err != nil {
			return domain.PriceCandidate{}, err
		}
		return candidate, nil
	}
	return manualPageCandidate(target, snapshot, fetchedAt)
}

func deepSeekTimedTokenRule(target application.OfficialPricePageTarget, rows []string, fetchedAt time.Time) (json.RawMessage, bool) {
	beijingYear := fetchedAt.UTC().Add(8 * time.Hour).Year()
	if target.AdapterKey != "deepseek" || target.Currency != "CNY" || target.Unit == nil ||
		*target.Unit != domain.PricePer1KTokens || fetchedAt.IsZero() || beijingYear != 2026 {
		return nil, false
	}
	if target.ProviderModelID != "deepseek-flash" && target.ProviderModelID != "deepseek-v4-pro" {
		return nil, false
	}

	modelColumn := -1
	headerIndex := -1
	for index, row := range rows {
		cells := strings.Split(row, " | ")
		matches := 0
		for column, cell := range cells {
			if rowMatchesModel(cell, normalizePriceModelName(target.ProviderModelID)) {
				modelColumn = column
				matches++
			}
		}
		if matches > 1 {
			return nil, false
		}
		if matches == 1 {
			headerIndex = index
			break
		}
	}
	if headerIndex < 0 || modelColumn <= 0 || len(rows) >= 20 {
		return nil, false
	}
	header := strings.Split(rows[headerIndex], " | ")
	type periodRates struct {
		peak    uint64
		offPeak uint64
	}
	rates := map[string]*periodRates{
		"input":  {},
		"cached": {},
		"output": {},
	}
	seen := make(map[string]struct{}, 6)
	for _, row := range rows[headerIndex+1:] {
		cells := strings.Split(row, " | ")
		if len(cells) != len(header) || len(cells) <= modelColumn {
			return nil, false
		}
		label := strings.Join(cells[:modelColumn], " ")
		if !strings.Contains(label, "输入") && !strings.Contains(label, "输出") {
			continue
		}
		if !strings.Contains(label, "百万") || !strings.Contains(strings.ToLower(label), "token") {
			return nil, false
		}
		var dimension string
		switch {
		case strings.Contains(label, "输出") && !strings.Contains(label, "输入"):
			dimension = "output"
		case strings.Contains(label, "输入") && strings.Contains(label, "缓存未命中"):
			dimension = "input"
		case strings.Contains(label, "输入") && strings.Contains(label, "缓存命中"):
			dimension = "cached"
		default:
			return nil, false
		}
		var period string
		switch {
		case strings.Contains(label, "高峰时段"):
			period = "peak"
		case strings.Contains(label, "空闲时段"):
			period = "off_peak"
		default:
			return nil, false
		}
		key := dimension + ":" + period
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		value, ok := yuanPerMillionTokensMicros(cells[modelColumn])
		if !ok || value == 0 {
			return nil, false
		}
		seen[key] = struct{}{}
		if period == "peak" {
			rates[dimension].peak = value
		} else {
			rates[dimension].offPeak = value
		}
	}
	if len(seen) != 6 {
		return nil, false
	}
	for _, rate := range rates {
		if rate.peak == 0 || rate.offPeak == 0 || rate.offPeak*2 != rate.peak {
			return nil, false
		}
	}
	rule, err := json.Marshal(struct {
		InputMicrosPer1K       uint64 `json:"input_micros_per_1k"`
		CachedInputMicrosPer1K uint64 `json:"cached_input_micros_per_1k"`
		OutputMicrosPer1K      uint64 `json:"output_micros_per_1k"`
		TimePricing            string `json:"time_pricing"`
	}{
		InputMicrosPer1K:       rates["input"].peak,
		CachedInputMicrosPer1K: rates["cached"].peak,
		OutputMicrosPer1K:      rates["output"].peak,
		TimePricing:            domain.DeepSeekCN2026,
	})
	return rule, err == nil
}

func yuanPerMillionTokensMicros(raw string) (uint64, bool) {
	value := strings.NewReplacer("人民币", "", "元/百万tokens", "", "元", "", "¥", "", "￥", "", string(rune(0x200b)), "", string(rune(0xfeff)), "").Replace(raw)
	value = strings.TrimSpace(value)
	if !decimalRatePattern.MatchString(value) {
		return 0, false
	}
	rate, ok := new(big.Rat).SetString(value)
	if !ok || rate.Sign() < 0 {
		return 0, false
	}
	micros := new(big.Rat).Mul(rate, new(big.Rat).SetInt64(1_000))
	if !micros.IsInt() || !micros.Num().IsUint64() || micros.Num().Uint64() > 9_007_199_254_740_991 {
		return 0, false
	}
	return micros.Num().Uint64(), true
}

func seedreamFlashRule(target application.OfficialPricePageTarget, rows []string) (json.RawMessage, bool) {
	if target.AdapterKey != "volcengine_ark" || target.Currency != "CNY" ||
		!strings.HasPrefix(target.ProviderModelID, "doubao-seedream-5-0-flash-") ||
		!strings.Contains(strings.Join(rows, " "), "输入图单价") ||
		!strings.Contains(strings.Join(rows, " "), "输出图单价") {
		return nil, false
	}
	var outputMicros uint64
	matched := 0
	for _, row := range rows {
		cells := strings.Split(row, " | ")
		if len(cells) != 3 || !rowMatchesModel(cells[0], normalizePriceModelName(target.ProviderModelID)) {
			continue
		}
		inputPrice := cleanPageText(cells[1])
		if !strings.Contains(inputPrice, "免费") && !strings.Contains(strings.ToLower(inputPrice), "free") {
			return nil, false
		}
		amount, ok := yuanPerImageMicros(cells[2])
		if !ok || amount == 0 {
			return nil, false
		}
		outputMicros = amount
		matched++
	}
	if matched != 1 {
		return nil, false
	}
	rule, err := json.Marshal(struct {
		Strategy string `json:"pricing_strategy"`
		Base     uint64 `json:"base_micros"`
	}{Strategy: domain.PricingImageFlatV1, Base: outputMicros})
	return rule, err == nil
}

func yuanPerImageMicros(raw string) (uint64, bool) {
	value := strings.NewReplacer("人民币", "", "元/张", "", "元", "", "¥", "", "￥", "", string(rune(0x200b)), "", string(rune(0xfeff)), "").Replace(raw)
	value = strings.TrimSpace(value)
	if !decimalRatePattern.MatchString(value) {
		return 0, false
	}
	rate, ok := new(big.Rat).SetString(value)
	if !ok || rate.Sign() < 0 {
		return 0, false
	}
	amount := new(big.Rat).Mul(rate, new(big.Rat).SetInt64(1_000_000))
	if !amount.IsInt() || !amount.Num().IsUint64() || amount.Num().Uint64() > 9_007_199_254_740_991 {
		return 0, false
	}
	return amount.Num().Uint64(), true
}

func cleanPageText(value string) string {
	return strings.TrimSpace(strings.NewReplacer(string(rune(0x200b)), "", string(rune(0xfeff)), "").Replace(value))
}

func manualPageCandidate(target application.OfficialPricePageTarget, snapshot []byte, fetchedAt time.Time) (domain.PriceCandidate, error) {
	hash := sha256.Sum256(snapshot)
	hexHash := hex.EncodeToString(hash[:])
	candidate := domain.PriceCandidate{
		ID:              uuid.NewSHA1(uuid.NameSpaceURL, []byte("glowloom:official-page:"+target.AdapterKey+":"+target.ProviderModelID+":"+officialPageParserV1+":"+hexHash)),
		AdapterKey:      target.AdapterKey,
		ProviderModelID: target.ProviderModelID,
		SourceURL:       target.SourceURL,
		SourceHash:      hexHash,
		ParserVersion:   officialPageParserV1,
		SourceFetchedAt: fetchedAt.UTC(),
		Currency:        target.Currency,
		Unit:            target.Unit,
		SourcePricing:   snapshot,
		Status:          domain.PriceCandidateNeedsReview,
	}
	if err := candidate.Validate(); err != nil {
		return domain.PriceCandidate{}, err
	}
	return candidate, nil
}

func officialPriceHost(host string) bool {
	switch strings.ToLower(host) {
	case "docs.volcengine.com", "api-docs.deepseek.com", "developers.openai.com":
		return true
	default:
		return false
	}
}

func extractHTMLTables(body []byte) [][][]string {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	var tables [][][]string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "table" {
			rows := tableRows(node)
			if len(rows) > 0 {
				tables = append(tables, rows)
			}
			return
		}
		if node.Type == html.ElementNode && (node.Data == "script" || node.Data == "style" || node.Data == "noscript") {
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return tables
}

func tableRows(table *html.Node) [][]string {
	var rows [][]string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "tr" {
			var cells []string
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode && (child.Data == "td" || child.Data == "th") {
					text := strings.Join(strings.Fields(nodeText(child)), " ")
					if text != "" {
						cells = append(cells, text)
					}
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		if node.Type == html.ElementNode && node.Data == "table" && node != table {
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(table)
	return rows
}

func nodeText(node *html.Node) string {
	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.ElementNode && (current.Data == "script" || current.Data == "style" || current.Data == "noscript") {
			return
		}
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
			builder.WriteByte(' ')
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return builder.String()
}

func matchingPriceRows(tables [][][]string, modelID string) []string {
	alias := normalizePriceModelName(modelID)
	if alias == "" {
		return nil
	}
	matched := make([]string, 0)
	seen := make(map[string]struct{})
	for _, rows := range tables {
		if len(rows) == 0 {
			continue
		}
		modelColumnHeader := rowMatchesAnyCell(rows[0], alias)
		headerMatch := isModelHeader(rows[0]) && modelColumnHeader
		tableHasMatch := modelColumnHeader
		if !tableHasMatch {
			for _, row := range rows[1:] {
				if rowMatchesAnyCell(row, alias) {
					tableHasMatch = true
					break
				}
			}
		}
		if !tableHasMatch {
			continue
		}
		if isModelHeader(rows[0]) || modelColumnHeader {
			appendDistinctRow(&matched, seen, rows[0])
		}
		for index, row := range rows {
			if headerMatch || modelColumnHeader && index > 0 || index > 0 && rowMatchesAnyCell(row, alias) {
				if modelColumnHeader && index > 0 {
					matched = append(matched, strings.Join(row, " | "))
				} else {
					appendDistinctRow(&matched, seen, row)
				}
			}
		}
	}
	if len(matched) > 20 {
		matched = matched[:20]
	}
	for index, line := range matched {
		if len(line) > 1000 {
			matched[index] = line[:1000]
		}
	}
	return matched
}

func appendDistinctRow(rows *[]string, seen map[string]struct{}, cells []string) {
	line := strings.Join(cells, " | ")
	if _, exists := seen[line]; exists {
		return
	}
	seen[line] = struct{}{}
	*rows = append(*rows, line)
}

func rowMatchesAnyCell(cells []string, alias string) bool {
	for _, cell := range cells {
		if rowMatchesModel(cell, alias) {
			return true
		}
	}
	return false
}

func rowMatchesModel(value, alias string) bool {
	normalized := normalizePriceModelName(value)
	if normalized == alias {
		return true
	}
	if !strings.HasPrefix(normalized, alias) {
		return false
	}
	// DeepSeek's price table annotates the model name with a one-digit footnote.
	suffix := strings.TrimPrefix(normalized, alias)
	return len(suffix) == 1 && suffix[0] >= '1' && suffix[0] <= '9'
}

func normalizePriceModelName(value string) string {
	value = strings.ToLower(value)
	if len(value) > 7 {
		suffix := value[len(value)-7:]
		if suffix[0] == '-' && allDecimalDigits(suffix[1:]) {
			value = value[:len(value)-7]
		}
	}
	var normalized strings.Builder
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			normalized.WriteRune(character)
		}
	}
	return normalized.String()
}

func allDecimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func isModelHeader(row []string) bool {
	if len(row) == 0 {
		return false
	}
	headerText := strings.ReplaceAll(row[0], string(rune(0x200b)), "")
	headerText = strings.ReplaceAll(headerText, string(rune(0x200c)), "")
	headerText = strings.ReplaceAll(headerText, string(rune(0x200d)), "")
	headerText = strings.ReplaceAll(headerText, string(rune(0xfeff)), "")
	header := strings.ToLower(strings.Join(strings.Fields(headerText), " "))
	return header == "模型" || header == "model" || header == "模型名称" || header == "model name"
}
