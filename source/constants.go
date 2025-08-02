package main

// parkNames maps park IDs to their human-readable names
var parkNames = map[string]string{
	"7340550b-c14d-4def-80bb-acdb51d49a66": "Disneyland Park",
	"832fcd51-ea19-4e77-85c7-75d5843b127c": "Disney California Adventure Park",
	"75ea578a-adc8-4116-a54d-dccb60765ef9": "Magic Kingdom Park",
	"47f90d2c-e191-4239-a466-5892ef59a88b": "EPCOT",
	"288747d1-8b4f-4a64-867e-ea7c9b27bad8": "Disney's Hollywood Studios",
	"1c84a229-8862-4648-9c71-378ddd2c7693": "Disney's Animal Kingdom Theme Park",
	"bd0eb47b-2f02-4d4d-90fa-cb3a68988e3b": "Hong Kong Disneyland",
	"3cc919f1-d16d-43e0-8c3f-1dd269bd1a42": "Tokyo Disneyland",
	"67b290d5-3478-4f23-b601-2f8fb71ba803": "Tokyo DisneySea",
	"ddc4357c-c148-4b36-9888-07894fe75e83": "Shanghai Disneyland",
	"dae968d5-630d-4719-8b06-3d107e944401": "Disneyland Park (Paris)",
	"ca888437-ebb4-4d50-aed2-d227f7096968": "Walt Disney Studios Park",
	"bc4005c5-8c7e-41d7-b349-cdddf1796427": "Universal Studios Hollywood",
	"eb3f4560-2383-4a36-9152-6b3e5ed6bc57": "Universal Studios Florida",
	"267615cc-8943-4c2a-ae2c-5da728ca591f": "Universal Islands of Adventure",
	"12dbb85b-265f-44e6-bccf-f1faa17211fc": "Universal's Epic Universe",
}

// getParkName returns the human-readable name for a given park ID
// If the park ID is not found, it returns "Unknown Park"
func getParkName(parkID string) string {
	if name, exists := parkNames[parkID]; exists {
		return name
	}
	return "Unknown Park"
} 