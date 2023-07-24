package pdf

func Px2mm(px float64) float64 {
	return px * 0.3528
}

func Mm2px(mm float64) float64 {
	return mm / 0.3528
}
