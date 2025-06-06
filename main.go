package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/debug"
	"github.com/joho/godotenv"
	"googlemaps.github.io/maps"
)

// PropertyInfo struct para armazenar os dados do imóvel
type PropertyInfo struct {
	Address      string `json:"address"`
	RentPrice    string `json:"price"`
	Bedrooms     string `json:"bedrooms"`
	Bathrooms    string `json:"bathrooms"`
	PropertyType string `json:"propertyType"`
	Description  string `json:"description"`
	URL          string `json:"url"`
	Error        string `json:"error,omitempty"` // Campo para mensagens de erro

	// Informações de localização
	Coordinates struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"coordinates"`

	// Informações de segurança
	SafetyInfo struct {
		CrimeRate      float64 `json:"crimeRate"`
		SafetyRating   int     `json:"safetyRating"` // 1-10
		NearbyGardai   []POI   `json:"nearbyGardai"` // Estações de polícia próximas
		StreetLighting string  `json:"streetLighting"`
	} `json:"safetyInfo"`

	// Qualidade de vida
	QualityOfLife struct {
		TransportScore  int   `json:"transportScore"` // 1-10
		PublicTransport []POI `json:"publicTransport"`
		Amenities       []POI `json:"amenities"`     // Supermercados, farmácias, etc
		Entertainment   []POI `json:"entertainment"` // Pubs, restaurantes, etc
		WalkScore       int   `json:"walkScore"`     // 1-100
	} `json:"qualityOfLife"`

	// Análise de valor
	ValueAnalysis struct {
		AreaAveragePrice float64           `json:"areaAveragePrice"`
		PriceRating      int               `json:"priceRating"` // 1-10 (1 = muito caro, 10 = muito barato)
		PriceHistory     []PricePoint      `json:"priceHistory"`
		Similar          []SimilarProperty `json:"similar"`
	} `json:"valueAnalysis"`
}

// POI (Point of Interest) representa um local de interesse próximo
type POI struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Distance float64 `json:"distance"` // em metros
	Duration int     `json:"duration"` // tempo de caminhada em minutos
}

// PricePoint representa um ponto no histórico de preços
type PricePoint struct {
	Date  string  `json:"date"`
	Price float64 `json:"price"`
}

// SimilarProperty representa um imóvel similar na região
type SimilarProperty struct {
	Address string  `json:"address"`
	Price   float64 `json:"price"`
	URL     string  `json:"url"`
}

// AnalysisResponse representa a resposta completa da análise
type AnalysisResponse struct {
	Property   PropertyInfo `json:"property"`
	SafetyInfo struct {
		CrimeStats struct {
			Total     int     `json:"total"`
			PerCapita float64 `json:"perCapita"`
			Breakdown []struct {
				Type  string `json:"type"`
				Count int    `json:"count"`
			} `json:"breakdown"`
		} `json:"crimeStats"`
		NearbyGardai []struct {
			Name     string  `json:"name"`
			Distance float64 `json:"distance"` // em km
			Phone    string  `json:"phone,omitempty"`
		} `json:"nearbyGardai"`
		StreetLighting struct {
			Rating      int    `json:"rating"` // 1-10
			Description string `json:"description"`
		} `json:"streetLighting"`
		SafetyScore   int      `json:"safetyScore"` // 1-100
		SafetyFactors []string `json:"safetyFactors"`
		RiskFactors   []string `json:"riskFactors"`
	} `json:"safetyInfo"`
}

// Função principal que coordena todas as análises
func enrichPropertyInfo(property *PropertyInfo) error {
	// 1. Obter coordenadas do endereço
	if err := getCoordinates(property); err != nil {
		return fmt.Errorf("erro ao obter coordenadas: %w", err)
	}

	// 2. Obter informações de segurança
	if err := getSafetyInfo(property); err != nil {
		log.Printf("Aviso: erro ao obter informações de segurança: %v", err)
	}

	// 3. Obter informações de qualidade de vida
	if err := getQualityOfLife(property); err != nil {
		log.Printf("Aviso: erro ao obter informações de qualidade de vida: %v", err)
	}

	// 4. Analisar valor do imóvel
	if err := analyzeValue(property); err != nil {
		log.Printf("Aviso: erro ao analisar valor: %v", err)
	}

	return nil
}

// Obter coordenadas usando a API do Google Maps
func getCoordinates(property *PropertyInfo) error {
	apiKey := os.Getenv("GOOGLE_MAPS_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("GOOGLE_MAPS_API_KEY não definida")
	}

	client, err := maps.NewClient(maps.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("erro ao criar cliente do Google Maps: %w", err)
	}

	// Adicionar "Ireland" ao endereço para melhor precisão
	fullAddress := property.Address
	if !strings.Contains(strings.ToLower(fullAddress), "ireland") {
		fullAddress += ", Ireland"
	}

	r := &maps.GeocodingRequest{
		Address: fullAddress,
		Region:  "ie", // Código do país para Irlanda
	}

	resp, err := client.Geocode(context.Background(), r)
	if err != nil {
		return fmt.Errorf("erro ao geocodificar endereço: %w", err)
	}

	if len(resp) == 0 {
		return fmt.Errorf("nenhum resultado encontrado para o endereço")
	}

	property.Coordinates.Lat = resp[0].Geometry.Location.Lat
	property.Coordinates.Lng = resp[0].Geometry.Location.Lng

	log.Printf("Coordenadas encontradas: %f, %f", property.Coordinates.Lat, property.Coordinates.Lng)
	return nil
}

// Obter informações de segurança
func getSafetyInfo(property *PropertyInfo) error {
	analysis := AnalysisResponse{Property: *property}

	if err := findNearbyGardai(&analysis); err != nil {
		return err
	}
	if err := analyzeStreetLighting(&analysis); err != nil {
		return err
	}
	if err := getCrimeStats(&analysis); err != nil {
		return err
	}

	calculateSafetyScore(&analysis)

	property.SafetyInfo.CrimeRate = analysis.SafetyInfo.CrimeStats.PerCapita
	property.SafetyInfo.SafetyRating = analysis.SafetyInfo.SafetyScore / 10
	property.SafetyInfo.StreetLighting = analysis.SafetyInfo.StreetLighting.Description
	for _, g := range analysis.SafetyInfo.NearbyGardai {
		property.SafetyInfo.NearbyGardai = append(property.SafetyInfo.NearbyGardai, POI{
			Name:     g.Name,
			Type:     "garda_station",
			Distance: g.Distance,
			Duration: int(g.Distance * 1000 / 80),
		})
	}

	return nil
}

// Obter informações de qualidade de vida
func getQualityOfLife(property *PropertyInfo) error {
	apiKey := os.Getenv("GOOGLE_MAPS_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("GOOGLE_MAPS_API_KEY not set")
	}

	client, err := maps.NewClient(maps.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("error creating Google Maps client: %w", err)
	}

	// 1. Encontrar transporte público
	if err := findPublicTransport(property, client); err != nil {
		log.Printf("Warning: error finding public transport: %v", err)
	}

	// 2. Encontrar amenidades
	if err := findAmenities(property, client); err != nil {
		log.Printf("Warning: error finding amenities: %v", err)
	}

	// 3. Encontrar entretenimento
	if err := findEntertainment(property, client); err != nil {
		log.Printf("Warning: error finding entertainment: %v", err)
	}

	// 4. Calcular walkability score
	calculateWalkScore(property)

	return nil
}

// findPublicTransport encontra estações de transporte público próximas
func findPublicTransport(property *PropertyInfo, client *maps.Client) error {
	location := &maps.LatLng{
		Lat: property.Coordinates.Lat,
		Lng: property.Coordinates.Lng,
	}

	// Buscar estações de trem
	trainStations, err := searchNearbyPlacesFn(client, location, "train_station", 2000)
	if err != nil {
		return err
	}

	// Buscar pontos de ônibus
	busStops, err := searchNearbyPlacesFn(client, location, "bus_station", 1000)
	if err != nil {
		return err
	}

	// Combinar resultados
	for _, station := range append(trainStations, busStops...) {
		dist := calculateDistance(location.Lat, location.Lng,
			station.Geometry.Location.Lat, station.Geometry.Location.Lng)

		tType := ""
		if len(station.Types) > 0 {
			tType = station.Types[0]
		}
		transport := POI{
			Name:     station.Name,
			Type:     tType,
			Distance: dist,
			Duration: int(dist * 1000 / 80), // Estimativa: 80m/min caminhando
		}
		property.QualityOfLife.PublicTransport = append(property.QualityOfLife.PublicTransport, transport)
	}

	// Calcular score de transporte (1-10)
	score := 5 // Base score
	if len(property.QualityOfLife.PublicTransport) > 0 {
		nearestStation := property.QualityOfLife.PublicTransport[0]
		if nearestStation.Distance < 0.5 { // Menos de 500m
			score += 3
		} else if nearestStation.Distance < 1.0 { // Menos de 1km
			score += 2
		}
		if len(property.QualityOfLife.PublicTransport) > 1 {
			score += 2 // Bônus por ter múltiplas opções
		}
	}
	property.QualityOfLife.TransportScore = score

	return nil
}

// findAmenities encontra amenidades próximas (supermercados, farmácias, etc)
func findAmenities(property *PropertyInfo, client *maps.Client) error {
	location := &maps.LatLng{
		Lat: property.Coordinates.Lat,
		Lng: property.Coordinates.Lng,
	}

	// Lista de tipos de amenidades para buscar
	amenityTypes := []string{
		"supermarket",
		"pharmacy",
		"convenience_store",
		"shopping_mall",
		"bank",
		"hospital",
		"doctor",
	}

	for _, amenityType := range amenityTypes {
		places, err := searchNearbyPlacesFn(client, location, amenityType, 1500)
		if err != nil {
			log.Printf("Warning: error searching for %s: %v", amenityType, err)
			continue
		}

		for _, place := range places {
			dist := calculateDistance(location.Lat, location.Lng,
				place.Geometry.Location.Lat, place.Geometry.Location.Lng)

			amenity := POI{
				Name:     place.Name,
				Type:     amenityType,
				Distance: dist,
				Duration: int(dist * 1000 / 80), // Estimativa: 80m/min caminhando
			}
			property.QualityOfLife.Amenities = append(property.QualityOfLife.Amenities, amenity)
		}
	}

	return nil
}

// findEntertainment encontra locais de entretenimento próximos
func findEntertainment(property *PropertyInfo, client *maps.Client) error {
	location := &maps.LatLng{
		Lat: property.Coordinates.Lat,
		Lng: property.Coordinates.Lng,
	}

	// Lista de tipos de entretenimento para buscar
	entertainmentTypes := []string{
		"restaurant",
		"bar",
		"cafe",
		"movie_theater",
		"gym",
		"park",
	}

	for _, entType := range entertainmentTypes {
		places, err := searchNearbyPlacesFn(client, location, entType, 2000)
		if err != nil {
			log.Printf("Warning: error searching for %s: %v", entType, err)
			continue
		}

		for _, place := range places {
			dist := calculateDistance(location.Lat, location.Lng,
				place.Geometry.Location.Lat, place.Geometry.Location.Lng)

			entertainment := POI{
				Name:     place.Name,
				Type:     entType,
				Distance: dist,
				Duration: int(dist * 1000 / 80), // Estimativa: 80m/min caminhando
			}
			property.QualityOfLife.Entertainment = append(property.QualityOfLife.Entertainment, entertainment)
		}
	}

	return nil
}

// searchNearbyPlaces é uma função auxiliar para buscar lugares próximos
func searchNearbyPlaces(client *maps.Client, location *maps.LatLng, placeType string, radius uint) ([]maps.PlacesSearchResult, error) {
	r := &maps.NearbySearchRequest{
		Location: location,
		Radius:   radius,
		Keyword:  placeType,
		Language: "en",
	}

	resp, err := client.NearbySearch(context.Background(), r)
	if err != nil {
		return nil, fmt.Errorf("error searching nearby places: %w", err)
	}

	return resp.Results, nil
}

// searchNearbyPlacesFn is a variable so tests can replace the search implementation
var searchNearbyPlacesFn = searchNearbyPlaces

// calculateWalkScore calcula o score de caminhabilidade
func calculateWalkScore(property *PropertyInfo) {
	score := 50 // Base score

	// Fatores que aumentam o score
	amenitiesNearby := 0
	for _, amenity := range property.QualityOfLife.Amenities {
		if amenity.Distance < 1.0 { // Menos de 1km
			amenitiesNearby++
		}
	}

	entertainmentNearby := 0
	for _, ent := range property.QualityOfLife.Entertainment {
		if ent.Distance < 1.0 { // Menos de 1km
			entertainmentNearby++
		}
	}

	// Ajustar score baseado em amenidades próximas
	score += min(amenitiesNearby*5, 25)     // Máximo de 25 pontos por amenidades
	score += min(entertainmentNearby*5, 25) // Máximo de 25 pontos por entretenimento

	// Ajustar baseado em transporte público
	if property.QualityOfLife.TransportScore >= 7 {
		score += 10
	} else if property.QualityOfLife.TransportScore >= 5 {
		score += 5
	}

	// Garantir que o score está entre 0 e 100
	property.QualityOfLife.WalkScore = min(max(score, 0), 100)
}

// Funções auxiliares
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Analisar valor do imóvel
func analyzeValue(property *PropertyInfo) error {
	// 1. Encontrar imóveis similares
	if err := findSimilarProperties(property); err != nil {
		log.Printf("Warning: error finding similar properties: %v", err)
	}

	// 2. Calcular preço médio da área
	calculateAreaAveragePrice(property)

	// 3. Calcular rating de preço
	calculatePriceRating(property)

	// 4. Buscar histórico de preços
	if err := getPriceHistory(property); err != nil {
		log.Printf("Warning: error getting price history: %v", err)
	}

	return nil
}

func findSimilarProperties(property *PropertyInfo) error {
	slugify := func(s string) string {
		s = strings.ToLower(strings.ReplaceAll(s, " ", "-"))
		var b strings.Builder
		for _, r := range s {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
				b.WriteRune(r)
			}
		}
		return b.String()
	}

	// ---------- montar URL de busca ----------
	basePrice := extractPriceValue(property.RentPrice)
	minPrice := roundToNearest50(basePrice * 0.8)
	maxPrice := roundToNearest50(basePrice * 1.2)

	locSlug := slugify(extractLocationFromAddress(property.Address))
	searchURL := fmt.Sprintf(
		"https://www.daft.ie/sharing/%s?rentalPrice_from=%.0f&rentalPrice_to=%.0f",
		locSlug, minPrice, maxPrice)

	// ---------- colly ----------
	c := colly.NewCollector(
		colly.AllowedDomains("www.daft.ie", "daft.ie"),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		colly.Debugger(&debug.LogDebugger{}),
	)

	// HEADERS
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.5")
		r.Headers.Set("DNT", "1")
		log.Printf("Buscando similares: %s", r.URL.String())
	})

	c.OnHTML("script#__NEXT_DATA__", func(e *colly.HTMLElement) {
		type nextData struct {
			Props struct {
				PageProps struct {
					Adverts []struct {
						DisplayAddress string `json:"displayAddress"`
						Price          struct {
							Monthly int `json:"monthly"`
							Weekly  int `json:"weekly"`
						} `json:"price"`
						AdPath string `json:"adPath"`
					} `json:"adverts"`
				} `json:"pageProps"`
			} `json:"props"`
		}

		var data nextData
		if err := json.Unmarshal([]byte(e.Text), &data); err != nil {
			log.Printf("Erro ao decodificar __NEXT_DATA__: %v", err)
			return
		}

		for _, ad := range data.Props.PageProps.Adverts {
			price := ad.Price.Monthly
			if price == 0 {
				price = ad.Price.Weekly
			}
			if price == 0 {
				continue
			}

			property.ValueAnalysis.Similar = append(property.ValueAnalysis.Similar, SimilarProperty{
				Address: ad.DisplayAddress,
				Price:   float64(price),
				URL:     "https://www.daft.ie" + ad.AdPath,
			})
		}
	})

	// ---------- 2) fallback simples caso JSON falhe ----------
	c.OnHTML("li[data-testid^='result-']", func(e *colly.HTMLElement) {
    // URL
    href := e.ChildAttr("a[href^='/share/']", "href")
    if href == "" {
        return
    }

    // Endereço
    address := strings.TrimSpace(
        e.ChildText("div[data-tracking='srp_address'] p"))
    if address == "" {
        return
    }

    // Preço (ex.: "€650 per month")
    priceTxt := strings.TrimSpace(
        e.ChildText("div[data-tracking='srp_price'] p"))
    price := extractPriceValue(priceTxt)
    if price == 0 {
        return
    }

    property.ValueAnalysis.Similar = append(
        property.ValueAnalysis.Similar,
        SimilarProperty{
            Address: address,
            Price:   price,
            URL:     "https://www.daft.ie" + href,
        })
	})
	// ---------- erro / resposta ----------
	c.OnError(func(r *colly.Response, err error) {
		log.Printf("Erro ao buscar similares: status %d – %v", r.StatusCode, err)
	})

	if err := c.Visit(searchURL); err != nil {
		return fmt.Errorf("error visiting search page: %w", err)
	}

	// limitar a 5
	if len(property.ValueAnalysis.Similar) > 5 {
		property.ValueAnalysis.Similar = property.ValueAnalysis.Similar[:5]
	}

	log.Printf("Imóveis similares encontrados: %d", len(property.ValueAnalysis.Similar))
	return nil
}

// calculateAreaAveragePrice calcula o preço médio da área
func calculateAreaAveragePrice(property *PropertyInfo) {
	if len(property.ValueAnalysis.Similar) == 0 {
		return
	}

	var total float64
	count := 0

	// Calcular média dos preços similares
	for _, similar := range property.ValueAnalysis.Similar {
		if similar.Price > 0 {
			total += similar.Price
			count++
		}
	}

	if count > 0 {
		property.ValueAnalysis.AreaAveragePrice = total / float64(count)
	}
}

// calculatePriceRating calcula o rating de preço (1-10)
func calculatePriceRating(property *PropertyInfo) {
	if property.ValueAnalysis.AreaAveragePrice == 0 {
		return
	}

	currentPrice := extractPriceValue(property.RentPrice)
	avgPrice := property.ValueAnalysis.AreaAveragePrice

	// Calcular diferença percentual do preço médio
	priceDiff := ((avgPrice - currentPrice) / avgPrice) * 100

	// Converter diferença em rating (1-10)
	// Quanto mais barato em relação à média, maior o rating
	switch {
	case priceDiff >= 20: // 20% ou mais abaixo da média
		property.ValueAnalysis.PriceRating = 10
	case priceDiff >= 15:
		property.ValueAnalysis.PriceRating = 9
	case priceDiff >= 10:
		property.ValueAnalysis.PriceRating = 8
	case priceDiff >= 5:
		property.ValueAnalysis.PriceRating = 7
	case priceDiff >= 0:
		property.ValueAnalysis.PriceRating = 6
	case priceDiff >= -5:
		property.ValueAnalysis.PriceRating = 5
	case priceDiff >= -10:
		property.ValueAnalysis.PriceRating = 4
	case priceDiff >= -15:
		property.ValueAnalysis.PriceRating = 3
	case priceDiff >= -20:
		property.ValueAnalysis.PriceRating = 2
	default: // Mais de 20% acima da média
		property.ValueAnalysis.PriceRating = 1
	}
}

// getPriceHistory busca histórico de preços do imóvel
func getPriceHistory(property *PropertyInfo) error {
	c := colly.NewCollector(
		colly.AllowedDomains("www.daft.ie", "daft.ie"),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		colly.Debugger(&debug.LogDebugger{}),
	)

	c.OnRequest(func(r *colly.Request) {
		log.Printf("Buscando histórico de preços: %s", r.URL.String())
	})

	c.OnResponse(func(r *colly.Response) {
		log.Printf("Status do histórico: %d", r.StatusCode)
		log.Printf("Content-Type: %s", r.Headers.Get("Content-Type"))
	})

	c.OnHTML("div[data-testid='price-history'] table", func(e *colly.HTMLElement) {
		log.Printf("Encontrou tabela de histórico")

		e.ForEach("tr", func(_ int, row *colly.HTMLElement) {
			date := row.ChildText("td:first-child")
			price := row.ChildText("td:last-child")

			log.Printf("Histórico encontrado - Data: %s, Preço: %s", date, price)

			if date != "" && price != "" {
				pricePoint := PricePoint{
					Date:  date,
					Price: extractPriceValue(price),
				}
				property.ValueAnalysis.PriceHistory = append(property.ValueAnalysis.PriceHistory, pricePoint)
			}
		})
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Printf("Erro ao buscar histórico: %v", err)
		log.Printf("Status code: %d", r.StatusCode)
	})

	err := c.Visit(property.URL)
	if err != nil {
		return fmt.Errorf("error visiting property page: %w", err)
	}

	return nil
}

// extractPriceValue extrai o valor numérico de uma string de preço
func extractPriceValue(price string) float64 {
	// Remover símbolos de moeda e separadores
	price = strings.TrimSpace(price)
	price = strings.ReplaceAll(price, "€", "")
	price = strings.ReplaceAll(price, ",", "")
	price = strings.ReplaceAll(price, " ", "")

	// Capturar apenas os dígitos iniciais (ignorando qualquer texto)
	re := regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?`)
	match := re.FindString(price)
	if match == "" {
		return 0
	}

	value, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return 0
	}
	return value
}

// extractLocationFromAddress extrai a localização principal do endereço
func extractLocationFromAddress(addr string) string {
	parts := strings.Split(addr, ",")
	for i := range parts {
		parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
	}

	// limpa vazios
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return ""
	}

	// heurística: penúltimo = bairro/subúrbio, último = condado / "dublin X"
	var suburb, county string
	if len(filtered) >= 2 {
		suburb = filtered[len(filtered)-2]
	}
	county = filtered[len(filtered)-1]

	// remove prefixos "co.", "county"
	county = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(county, "co."), "county"))

	// se o condado tiver números (ex.: "dublin 9"), pega só a 1ª palavra (⇒ "dublin")
	if idx := strings.IndexFunc(county, unicode.IsDigit); idx != -1 {
		county = strings.Fields(county)[0]
	}

	// gera slug preservando letras e hífens (sem números)
	return slugify(suburb) + "-" + slugify(county)
}

// roundToNearest50 arredonda um número para o múltiplo de 50 mais próximo
func roundToNearest50(value float64) float64 {
	return math.Round(value/50) * 50
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, " ", "-")))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// scrapeDaftProperty raspa os dados de um anúncio do Daft.ie
func scrapeDaftProperty(url string) (PropertyInfo, error) {
	c := colly.NewCollector(
		colly.AllowedDomains("www.daft.ie", "daft.ie"),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		colly.AllowURLRevisit(),
		colly.Debugger(&debug.LogDebugger{}),
	)

	// Configurar headers adicionais
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.5")
		r.Headers.Set("Cache-Control", "no-cache")
		r.Headers.Set("Pragma", "no-cache")
		r.Headers.Set("DNT", "1")
		r.Headers.Set("Connection", "keep-alive")
		r.Headers.Set("Upgrade-Insecure-Requests", "1")
		log.Printf("Fazendo requisição para: %s", r.URL.String())
	})

	property := PropertyInfo{URL: url}
	foundAddress := false

	// Debug: Imprimir HTML antes do parsing
	c.OnResponse(func(r *colly.Response) {
		log.Printf("Status: %d", r.StatusCode)
		log.Printf("Content-Type: %s", r.Headers.Get("Content-Type"))
		log.Printf("Body length: %d", len(r.Body))

		// Salvar HTML para debug
		err := r.Save("debug_response.html")
		if err != nil {
			log.Printf("Erro ao salvar HTML: %v", err)
		}
	})

	// Encontrar o endereço
	c.OnHTML("meta[property='og:title']", func(e *colly.HTMLElement) {
		if !foundAddress {
			text := strings.TrimSpace(e.Attr("content"))
			if text != "" && strings.Contains(text, "to share on Daft.ie") {
				text = strings.TrimSuffix(text, " to share on Daft.ie")
				log.Printf("Encontrou endereço (meta): %s", text)
				property.Address = text
				foundAddress = true
			}
		}
	})

	// Encontrar o preço
	c.OnHTML("meta[property='og:description']", func(e *colly.HTMLElement) {
		if property.RentPrice == "" {
			text := strings.TrimSpace(e.Attr("content"))
			if strings.Contains(text, "€") {
				priceStart := strings.Index(text, "€")
				priceEnd := strings.Index(text[priceStart:], " per")
				if priceEnd > 0 {
					price := text[priceStart : priceStart+priceEnd]
					log.Printf("Encontrou preço (meta): %s", price)
					property.RentPrice = price
				}
			}
		}
	})

	// Encontrar características do imóvel
	c.OnHTML("[data-testid='features'], [data-testid='overview'], ul[class*='PropertyFeatures'], ul[class*='PropertyOverview']", func(e *colly.HTMLElement) {
		e.ForEach("li", func(_ int, item *colly.HTMLElement) {
			text := strings.ToLower(strings.TrimSpace(item.Text))
			log.Printf("Analisando característica: %s", text)

			if strings.Contains(text, "bed") || strings.Contains(text, "bedroom") {
				property.Bedrooms = text
				log.Printf("Encontrou quartos: %s", text)
			} else if strings.Contains(text, "bath") {
				property.Bathrooms = text
				log.Printf("Encontrou banheiros: %s", text)
			} else if strings.Contains(text, "property type") || strings.Contains(text, "type:") {
				property.PropertyType = text
				log.Printf("Encontrou tipo: %s", text)
			}
		})
	})

	// Encontrar descrição
	c.OnHTML("[data-testid='description'], div[class*='PropertyDescription']", func(e *colly.HTMLElement) {
		if property.Description == "" {
			text := strings.TrimSpace(e.Text)
			if text != "" {
				log.Printf("Encontrou descrição: %s", text)
				property.Description = text
			}
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Printf("Erro ao acessar %s: %v", r.Request.URL, err)
		log.Printf("Status code: %d", r.StatusCode)
		log.Printf("Headers: %v", r.Headers)
		if r.StatusCode == 403 {
			property.Error = "Acesso bloqueado pelo site. Tente novamente mais tarde."
		} else {
			property.Error = fmt.Sprintf("Erro ao acessar a página: %v", err)
		}
	})

	// Configurar limite de requisições
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*daft.ie*",
		Delay:       2 * time.Second,
		RandomDelay: 1 * time.Second,
	})

	err := c.Visit(url)
	if err != nil {
		return PropertyInfo{}, fmt.Errorf("failed to visit URL: %w", err)
	}

	// Verificar se os dados essenciais foram encontrados
	if !foundAddress || property.RentPrice == "" {
		if property.Error == "" {
			property.Error = "Could not find essential property data. The page structure might have changed or it's not a property listing."
		}
	}

	// Após obter os dados básicos, enriquecer com informações adicionais
	if err := enrichPropertyInfo(&property); err != nil {
		log.Printf("Aviso: erro ao enriquecer informações: %v", err)
	}

	return property, nil
}

// handleScrape é o handler HTTP para a rota de scraping
func handleScrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var requestBody struct {
		DaftURL string `json:"daftUrl"`
	}

	err := json.NewDecoder(r.Body).Decode(&requestBody)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if requestBody.DaftURL == "" {
		http.Error(w, "daftUrl is required in the request body", http.StatusBadRequest)
		return
	}

	log.Printf("Received request to scrape: %s", requestBody.DaftURL)

	property, scrapeErr := scrapeDaftProperty(requestBody.DaftURL)
	if scrapeErr != nil {
		log.Printf("Scraping error: %v", scrapeErr)
		http.Error(w, fmt.Sprintf("Error during scraping: %v", scrapeErr), http.StatusInternalServerError)
		return
	}

	// Se houver um erro dentro da struct PropertyInfo, significa que o scraping falhou em encontrar dados.
	if property.Error != "" {
		log.Printf("Scraping data extraction error: %s", property.Error)
		// Você pode decidir retornar um 200 OK com o erro na resposta JSON ou um 500 Internal Server Error.
		// Por enquanto, vamos retornar 200 OK com o erro no JSON para o cliente poder decidir como lidar.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(property)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(property)
}

// handleAnalyze é o handler HTTP para a rota de análise completa
func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var requestBody struct {
		DaftURL string `json:"daftUrl"`
	}

	err := json.NewDecoder(r.Body).Decode(&requestBody)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if requestBody.DaftURL == "" {
		http.Error(w, "daftUrl is required in the request body", http.StatusBadRequest)
		return
	}

	log.Printf("Received request to analyze: %s", requestBody.DaftURL)

	// 1. Primeiro fazer o scraping básico
	property, err := scrapeDaftProperty(requestBody.DaftURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error during scraping: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. Criar a resposta da análise
	analysis := AnalysisResponse{
		Property: property,
	}

	// 3. Obter coordenadas do endereço
	if err := getCoordinates(&analysis.Property); err != nil {
		log.Printf("Warning: failed to get coordinates: %v", err)
	}

	// 4. Analisar segurança
	if err := analyzeSafety(&analysis); err != nil {
		log.Printf("Warning: failed to analyze safety: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analysis)
}

// analyzeSafety analisa a segurança da região
func analyzeSafety(analysis *AnalysisResponse) error {
	// 1. Buscar delegacias próximas usando Google Places API
	if err := findNearbyGardai(analysis); err != nil {
		return fmt.Errorf("error finding nearby Gardai: %w", err)
	}

	// 2. Analisar iluminação pública usando dados do OpenStreetMap
	if err := analyzeStreetLighting(analysis); err != nil {
		return fmt.Errorf("error analyzing street lighting: %w", err)
	}

	// 3. Obter estatísticas de crime da região
	if err := getCrimeStats(analysis); err != nil {
		return fmt.Errorf("error getting crime stats: %w", err)
	}

	// 4. Calcular score de segurança
	calculateSafetyScore(analysis)

	return nil
}

// findNearbyGardai encontra delegacias próximas usando Google Places API
func findNearbyGardai(analysis *AnalysisResponse) error {
	apiKey := os.Getenv("GOOGLE_MAPS_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("GOOGLE_MAPS_API_KEY not set")
	}

	client, err := maps.NewClient(maps.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("error creating Google Maps client: %w", err)
	}

	location := &maps.LatLng{
		Lat: analysis.Property.Coordinates.Lat,
		Lng: analysis.Property.Coordinates.Lng,
	}

	r := &maps.NearbySearchRequest{
		Location: location,
		Radius:   5000, // 5km
		Keyword:  "garda station police",
		Language: "en",
	}

	resp, err := client.NearbySearch(context.Background(), r)
	if err != nil {
		return fmt.Errorf("error searching nearby places: %w", err)
	}

	for _, place := range resp.Results {
		station := struct {
			Name     string  `json:"name"`
			Distance float64 `json:"distance"`
			Phone    string  `json:"phone,omitempty"`
		}{
			Name:     place.Name,
			Distance: calculateDistance(location.Lat, location.Lng, place.Geometry.Location.Lat, place.Geometry.Location.Lng),
		}
		analysis.SafetyInfo.NearbyGardai = append(analysis.SafetyInfo.NearbyGardai, station)
	}

	return nil
}

// analyzeStreetLighting analisa a iluminação pública usando OpenStreetMap
func analyzeStreetLighting(analysis *AnalysisResponse) error {
	query := fmt.Sprintf(`[out:json];node["highway"="street_lamp"](around:500,%f,%f);out count;`,
		analysis.Property.Coordinates.Lat, analysis.Property.Coordinates.Lng)

	resp, err := http.PostForm("https://overpass-api.de/api/interpreter",
		url.Values{"data": {query}})
	if err != nil {
		return fmt.Errorf("error querying Overpass API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("overpass API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Elements []struct {
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("error decoding overpass response: %w", err)
	}

	count := 0
	if len(result.Elements) > 0 {
		if v, ok := result.Elements[0].Tags["nodes"]; ok {
			count, _ = strconv.Atoi(v)
		}
	}

	rating := 4
	switch {
	case count > 50:
		rating = 10
	case count > 20:
		rating = 8
	case count > 10:
		rating = 6
	}

	analysis.SafetyInfo.StreetLighting.Rating = rating
	analysis.SafetyInfo.StreetLighting.Description = fmt.Sprintf("%d street lights within 500m", count)
	return nil
}

// getCrimeStats obtém estatísticas de crime da região
func getCrimeStats(analysis *AnalysisResponse) error {
	// 1. Consulta CSO
	stats, err := GetCrimeStats(
		analysis.Property.Coordinates.Lat,
		analysis.Property.Coordinates.Lng,
	)
	if err != nil {
		return fmt.Errorf("error getting crime stats: %w", err)
	}

	// 2. Copia total e per-capita
	analysis.SafetyInfo.CrimeStats.Total = stats.Total
	analysis.SafetyInfo.CrimeStats.PerCapita = stats.PerCapita

	// 3. Converte []CrimeTypeData → slice anônimo esperado pelo JSON
	if len(stats.Breakdown) == 0 {
		analysis.SafetyInfo.CrimeStats.Breakdown = []struct {
			Type  string `json:"type"`
			Count int    `json:"count"`
		}{}
		return nil
	}

	converted := make([]struct {
		Type  string `json:"type"`
		Count int    `json:"count"`
	}, len(stats.Breakdown))

	for i, ct := range stats.Breakdown {
		converted[i].Type = ct.Type
		converted[i].Count = ct.Count
	}

	analysis.SafetyInfo.CrimeStats.Breakdown = converted
	return nil
}

// calculateSafetyScore calcula o score de segurança
func calculateSafetyScore(analysis *AnalysisResponse) {
	// Fatores positivos
	analysis.SafetyInfo.SafetyFactors = []string{}
	if len(analysis.SafetyInfo.NearbyGardai) > 0 {
		analysis.SafetyInfo.SafetyFactors = append(analysis.SafetyInfo.SafetyFactors,
			fmt.Sprintf("Garda station within %.1f km", analysis.SafetyInfo.NearbyGardai[0].Distance))
	}
	if analysis.SafetyInfo.StreetLighting.Rating >= 7 {
		analysis.SafetyInfo.SafetyFactors = append(analysis.SafetyInfo.SafetyFactors,
			"Well-lit streets")
	}

	// Fatores de risco
	analysis.SafetyInfo.RiskFactors = []string{}
	if analysis.SafetyInfo.CrimeStats.PerCapita > 0.02 {
		analysis.SafetyInfo.RiskFactors = append(analysis.SafetyInfo.RiskFactors,
			"Above average crime rate")
	}

	// Calcular score final (1-100)
	score := 70 // Base score

	// Ajustar baseado em fatores
	score += len(analysis.SafetyInfo.SafetyFactors) * 5
	score -= len(analysis.SafetyInfo.RiskFactors) * 10
	score += analysis.SafetyInfo.StreetLighting.Rating * 2

	// Garantir que está entre 1-100
	if score < 1 {
		score = 1
	} else if score > 100 {
		score = 100
	}

	analysis.SafetyInfo.SafetyScore = score
}

func init() {
	// Carregar variáveis de ambiente do arquivo .env
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, using system environment variables")
	}

	// Verificar se a chave da API está definida
	if os.Getenv("GOOGLE_MAPS_API_KEY") == "" {
		log.Printf("Warning: GOOGLE_MAPS_API_KEY not set, some features will be disabled")
	}
}

func main() {
	http.HandleFunc("/scrape", handleScrape)
	http.HandleFunc("/analyze", handleAnalyze)
	port := ":8080"
	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
