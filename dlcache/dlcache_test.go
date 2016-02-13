package dlcache

import "testing"

func Test_dl_cache_libcmp(t *testing.T) {

	t.Log(_dl_cache_libcmp("a.so", "b.so"))
	t.Log(_dl_cache_libcmp("b.so", "a.so"))
	t.Log(_dl_cache_libcmp("c.so", "c.so"))
	t.Log(_dl_cache_libcmp("a-1.so", "a-2.so"))
	t.Log(_dl_cache_libcmp("a-1.so", "a-10.so"))
	t.Log(_dl_cache_libcmp("a-10.so", "a-1.so"))
	t.Log(_dl_cache_libcmp("a-10.so", "a-10.so"))
	t.Log(_dl_cache_libcmp("libm.so.6", "libm.so"))
}
