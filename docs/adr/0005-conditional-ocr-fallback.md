# Conditional OCR Fallback

OCR is a fallback path rather than the default extraction path. The document service first attempts native extraction and calls the OCR service only for image files, scanned PDF pages, or extracted regions whose native text quality is too low, preserving CPU performance for ordinary office documents while still supporting scanned material.

