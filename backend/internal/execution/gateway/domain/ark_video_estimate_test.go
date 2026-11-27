package domain

import "testing"

func TestArkVideoEstimateFloorsPerOutputAndRoundsAfterCounting(t *testing.T) {
	profile := ArkVideoEstimateProfile{
		FramesPerSecond: 24, PixelsPerToken: 1024,
		ReferenceFloorNumerator: 2, ReferenceFloorDenominator: 3,
		MinReferenceSeconds: 2, MaxReferenceSeconds: 30,
		MinOutputSeconds: 4, MaxOutputSeconds: 30,
		Resolutions: map[string]ArkVideoEstimateRate{
			"720p": {
				CNYPerMillionWithoutVideo: 70, CNYPerMillionWithVideo: 42,
				Geometries: map[string]VideoGeometry{"16:9": {Width: 1280, Height: 720}},
			},
		},
	}
	for _, tc := range []struct {
		name                     string
		reference, output, count int64
		amount                   string
	}{
		{"no reference", 0, 5, 1, "7.56"},
		{"minimum reference applied separately", 2, 4, 2, "12.70"},
		{"rounding after total", 15, 15, 3, "81.65"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := profile.Estimate(ArkVideoEstimateInput{
				Resolution: "720p", Ratio: "16:9",
				ReferenceSeconds: tc.reference, OutputSeconds: tc.output, Count: tc.count,
			})
			if err != nil || got.AmountCNY != tc.amount {
				t.Fatalf("Estimate() = %+v, %v; want amount %s", got, err, tc.amount)
			}
		})
	}
}
