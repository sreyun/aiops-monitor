package main

import (
	"image"
	"image/color"
	"testing"
)

func TestIsLikelyUniformSolidBlue(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	blue := color.RGBA{R: 0, G: 0x20, B: 0x60, A: 255}
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, blue)
		}
	}
	if !isLikelyUniform(img, false) {
		t.Fatal("solid blue should be uniform")
	}
	if isLikelyUniform(img, true) {
		t.Fatal("solid blue should not match blackOnly")
	}
}

func TestIsLikelyUniformBlack(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{A: 255})
		}
	}
	if !isLikelyBlank(img) {
		t.Fatal("black should be blank")
	}
}

func TestIsLikelyUniformRealContent(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 5), B: 40, A: 255})
		}
	}
	if isLikelyUniform(img, false) {
		t.Fatal("gradient should not be uniform")
	}
}
