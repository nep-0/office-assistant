import unittest

from server import collect_values, fake_ocr, flatten, is_number


class OCRServerTest(unittest.TestCase):
    def test_fake_ocr_returns_text(self):
        result = fake_ocr(b"image-bytes", "scan.png")

        self.assertIn("scan.png", result["text"])
        self.assertEqual(result["engine"], "fake")

    def test_collect_values_reads_nested_ppocr_results(self):
        result = [{"res": {"rec_texts": ["Alpha", "Beta"], "rec_scores": [0.9, 0.8]}}]

        texts = list(flatten(collect_values(result, "rec_texts")))
        scores = list(flatten(collect_values(result, "rec_scores")))

        self.assertEqual(texts, ["Alpha", "Beta"])
        self.assertEqual(scores, [0.9, 0.8])
        self.assertTrue(is_number(scores[0]))


if __name__ == "__main__":
    unittest.main()
