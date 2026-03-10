"""验证 SRP 中间值 - 与 Go 实现对比"""
import hashlib

# RFC 5054 2048-bit prime (same as pysrp NG_2048)
N_hex = "AC6BDB41324A9A9BF166DE5E1389582FAF72B6651987EE07FC3192943DB56050A37329CBB4A099ED8193E0757767A13DD52312AB4B03310DCD7F48A9DA04FD50E8083969EDB767B0CF6095179A163AB3661A05FBD5FAAAE82918A9962F0B93B855F97993EC975EEAA80D740ADBF4FF747359D041D5C33EA71D281E446B14773BCA97B43A23FB801676BD207A436C6481F1D2B9078717461A5B9D32E688F87748544523B524B0D57D5EA77A2775D2ECFA032CFBDBF52FB3786160279004E57AE6AF874E7303CE53299CCC041C7BC308D82A5698F3A8D0C38271AE35F8E9DBFBB694B5C803D89F7AE435DE236D525F54759B65E372FCD68EF20FA7111F9E4AFF73"
g = 2

N = int(N_hex, 16)
N_bytes = N.to_bytes(256, 'big')
g_bytes = g.to_bytes(1, 'big')
g_padded = b'\x00' * (256 - 1) + g_bytes  # pad to 256 bytes

print(f"N_len = {len(N_bytes)}")
print(f"g_padded_len = {len(g_padded)}")

# k = H(N || pad(g))
k_bytes = hashlib.sha256(N_bytes + g_padded).digest()
k = int.from_bytes(k_bytes, 'big')
print(f"k_hex = {k_bytes.hex()}")

# hN = H(N)
hN = hashlib.sha256(N_bytes).digest()
print(f"hN = {hN[:8].hex()}")

# hg = H(pad(g))  -- pysrp always pads g in HNxorg
hg = hashlib.sha256(g_padded).digest()
print(f"hg = {hg[:8].hex()}")

# xorNG
xorNG = bytes(a ^ b for a, b in zip(hN, hg))
print(f"xorNG = {xorNG[:8].hex()}")

print("\n--- 以上是确定性值，应与 Go 日志完全一致 ---")
