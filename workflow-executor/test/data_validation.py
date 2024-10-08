import unittest

import executor.scheduling.types


class MyTestCase(unittest.TestCase):
    def test_something(self):
        print(executor.scheduling.types.Workflow.parse_file("./examples/simple.json"))
        # self.assertEqual(True, False)  # add assertion here
        self.assertEqual(True, True)  # add assertion here


if __name__ == '__main__':
    unittest.main()
