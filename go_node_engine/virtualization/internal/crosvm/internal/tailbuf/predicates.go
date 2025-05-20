package tailbuf

func IsValidUTF8Start(b byte) bool {
	// if the top two bits of a UTF-8 byte are '10', it is a continuation byte and therefore
	// not a valid start to a sequence of UTF-8 characters
	return (b & 0xC0) != 0x80
}
