package imagecache

// buckets are the widths a smaller copy may have; a requested width snaps up
// to the next one so arbitrary widths cannot fill the cache.
var buckets = []int{96, 160, 240, 320, 480, 600, 800, 1280}

// Bucket is the variant width to serve for a requested width w of an image
// stored at width stored; 0 means the stored original.
func Bucket(w, stored int) int {
	if w <= 0 {
		return 0
	}
	for _, b := range buckets {
		if b >= w {
			if b >= stored {
				return 0
			}
			return b
		}
	}
	return 0
}
