import unittest

from recover_history import report_samples


class HumidityRecoveryTest(unittest.TestCase):
    def test_declared_unit_interval_becomes_percent(self):
        report = {"measuredData": {"humidity": {
            "percentageUnit": "UNIT_INTERVAL",
            "dataPoints": [
                {"timestamp": "2026-11-07T00:00:00Z", "value": 0.465},
                {"timestamp": "2026-11-07T00:15:00Z", "value": 0.005},
                {"timestamp": "2026-11-07T00:30:00Z", "value": None},
                {"timestamp": "", "value": 0.5},
            ],
        }}}
        samples = list(report_samples(report, {}))
        self.assertEqual([value for _, _, value in samples], [46.5, 0.5])
        self.assertEqual([stamp for _, stamp, _ in samples], [1794009600000, 1794010500000])
        self.assertTrue(all(metric["__name__"] == "gohome_tado_history_humidity_percent" for metric, _, _ in samples))

    def test_percentage_is_not_scaled_by_magnitude(self):
        report = {"measuredData": {"humidity": {"dataPoints": [
            {"timestamp": "2026-11-07T00:00:00Z", "value": {"percentage": 0.5}},
            {"timestamp": "2026-11-07T00:15:00Z", "value": 0.5},
        ]}}}
        self.assertEqual([value for _, _, value in report_samples(report, {})], [0.5, 0.5])


if __name__ == "__main__":
    unittest.main()
