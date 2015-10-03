import unittest
import bmemcached
from bmemcached.compat import long, unicode


class MemcachedTests(unittest.TestCase):
    def setUp(self):
        self.server = '127.0.0.1:11211'
        # self.server = '/tmp/memcached.sock'
        self.client = bmemcached.Client(self.server)

    def tearDown(self):
        self.reset()
        self.client.disconnect_all()

    def reset(self):
        self.client.delete('test_key')
        self.client.delete('test_key2')

    def testSet(self):
        self.assertTrue(self.client.set('test_key', 'test'))

    def testGet(self):
        self.client.set('test_key', 'test')
        self.assertEqual('test', self.client.get('test_key'))

    def testGetEmptyString(self):
        self.client.set('test_key', '')
        self.assertEqual('', self.client.get('test_key'))

    def testGetUnicodeString(self):
        self.client.set('test_key', '\xac')
        self.assertEqual('\xac', self.client.get('test_key'))

    def testGetLong(self):
        self.client.set('test_key', long(1))
        value = self.client.get('test_key')
        self.assertEqual(long(1), value)
        self.assertTrue(isinstance(value, long))

    def testGetInteger(self):
        self.client.set('test_key', 1)
        value = self.client.get('test_key')
        self.assertEqual(1, value)
        self.assertTrue(isinstance(value, int))

    def testGetBoolean(self):
        self.client.set('test_key', True)
        self.assertTrue(self.client.get('test_key') is True)

    def testGetObject(self):
        self.client.set('test_key', {'a': 1})
        value = self.client.get('test_key')
        self.assertTrue(isinstance(value, dict))
        self.assertTrue('a' in value)
        self.assertEqual(1, value['a'])

    def testDelete(self):
        self.client.set('test_key', 'test')
        self.assertTrue(self.client.delete('test_key'))
        self.assertEqual(None, self.client.get('test_key'))

    def testDeleteUnknownKey(self):
        self.assertTrue(self.client.delete('test_key'))

    def testReconnect(self):
        self.client.set('test_key', 'test')
        self.client.disconnect_all()
        self.assertEqual('test', self.client.get('test_key'))

if __name__ == '__main__':
    unittest.main()
