#!/usr/bin/env python3
"""No radio hardware, multimon installation, or live network needed."""

import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import wave
import os
import stat
import sys


SCRIPT = Path(__file__).with_name("evaluate-aprs-wav.py")
spec = importlib.util.spec_from_file_location("aprs_wav_evaluator", SCRIPT)
evaluator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(evaluator)


class ReplayTests(unittest.TestCase):
    def test_extract_multimon_lines(self):
        output = (
            "Enabled demodulators: AFSK1200\n"
            "APRS: N0CALL>APRS:>status\n"
            "noise\n"
            "APRS: N0CALL>APRS:>status\n"
            "APRS: W7ABC>APRS:!position\n"
        )
        self.assertEqual(evaluator.decoded_frames(output), [
            "N0CALL>APRS:>status",
            "N0CALL>APRS:>status",
            "W7ABC>APRS:!position",
        ])

    def test_payload_whitespace_is_significant(self):
        plain = "N0CALL>APRS:>test"
        spaced = plain + " "
        self.assertEqual(evaluator.decoded_frames(
            "APRS: " + plain + "\r\nAPRS: " + spaced + "\n"
        ), [plain, spaced])
        self.assertEqual(evaluator.compare([spaced], [plain])["correct"], 0)
        self.expected.write_bytes((spaced + "\r\n").encode("utf-8"))
        args = ["--wav", str(self.wav), "--expected", str(self.expected),
                "--decoder", str(self.decoder), "--repeat", "1",
                "--output", str(self.root / "result.json")]
        self.assertEqual(evaluator.main(args), 1)

    def test_repeatability_compares_actual_multisets(self):
        with patch.object(evaluator, "run_decoder", side_effect=[
            (["A", "B"], 1.0), (["A", "C"], 1.0),
        ]):
            result = evaluator.evaluate(self.wav, ["A"], str(self.decoder), 2, 5)
        self.assertFalse(result["consistent_frames_across_runs"])
        self.assertEqual(result["runs"][0], result["runs"][1])
        with patch.object(evaluator, "run_decoder", side_effect=[
            (["A", "B", "A"], 1.0), (["B", "A", "A"], 1.0),
        ]):
            result = evaluator.evaluate(self.wav, ["A"], str(self.decoder), 2, 5)
        self.assertTrue(result["consistent_frames_across_runs"])

    def test_compare_repeated_and_unexpected_packets(self):
        stats = evaluator.compare(["A", "A", "B"], ["A", "B", "B"])
        self.assertEqual((stats["correct"], stats["missed"], stats["extra"]), (2, 1, 1))
        self.assertAlmostEqual(stats["recall"], 2 / 3)
        self.assertAlmostEqual(stats["precision"], 2 / 3)
        empty = evaluator.compare(["A"], [])
        self.assertIsNone(empty["precision"])
        self.assertEqual(empty["recall"], 0)

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.wav = self.root / "sample.wav"
        with wave.open(str(self.wav), "wb") as audio:
            audio.setnchannels(1)
            audio.setsampwidth(2)
            audio.setframerate(22050)
            audio.writeframes(bytes(22050 * 2))
        self.expected = self.root / "expected.txt"
        self.expected.write_text("N0CALL>APRS:>test\n", encoding="utf-8")
        self.decoder = self.root / "fake-multimon"
        self.decoder.write_text(
            "#!" + sys.executable + "\n"
            "import sys\n"
            "assert sys.argv[1:6] == ['-a', 'AFSK1200', '-A', '-t', 'wav']\n"
            "print('Enabled demodulators: AFSK1200')\n"
            "print('APRS: N0CALL>APRS:>test')\n",
            encoding="utf-8",
        )
        self.decoder.chmod(self.decoder.stat().st_mode | stat.S_IXUSR)

    def test_replay_success_and_json_contract(self):
        result = evaluator.evaluate(self.wav, ["N0CALL>APRS:>test"], str(self.decoder), 2, 5)
        self.assertEqual(result["format_version"], 1)
        self.assertEqual(result["repeats"], 2)
        self.assertEqual(result["audio"]["sample_rate_hz"], 22050)
        self.assertEqual(result["audio"]["duration_seconds"], 1.0)
        self.assertTrue(result["consistent_frames_across_runs"])
        self.assertTrue(all(run["correct"] == 1 and run["extra"] == 0 for run in result["runs"]))

    def test_cli_exit_codes_and_json_output(self):
        output = self.root / "result.json"
        good = ["--wav", str(self.wav), "--expected", str(self.expected),
                "--decoder", str(self.decoder), "--repeat", "2", "--output", str(output)]
        self.assertEqual(evaluator.main(good), 0)
        self.assertTrue(output.is_file())
        self.expected.write_text("OTHER>APRS:>not-present\n", encoding="utf-8")
        self.assertEqual(evaluator.main(good), 1)
        with self.assertRaises(SystemExit) as raised:
            evaluator.main(good + ["--timeout", "-1"])
        self.assertEqual(raised.exception.code, 2)

    def test_missing_audio_or_empty_ground_truth(self):
        self.expected.write_text("", encoding="utf-8")
        self.assertEqual(evaluator.main(["--wav", str(self.wav),
                                          "--expected", str(self.expected),
                                          "--decoder", str(self.decoder)]), 2)


if __name__ == "__main__":
    unittest.main()
