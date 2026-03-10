"""
Full SRP verification: compute ALL intermediate values with pysrp
and print them so we can compare with Go's srp_debug.log
"""
import hashlib
import srp._pysrp as srp

srp.rfc5054_enable()
srp.no_username_in_x()

# Known test values
username = "test@example.com"
derived_password = bytes.fromhex("5a4c18b281a7b878" + "00" * 24)  # 32 bytes placeholder
salt = bytes.fromhex("da3c2a7ae626ee4e84d9d06c00eee1eb")

# Get N, g
N, g = srp.get_ng(srp.NG_2048, None, None)
hash_class = hashlib.sha256

# Verify k
k_bytes = srp.H(hash_class, N, g, width=len(srp.long_to_bytes(N)))
k = srp.bytes_to_long(k_bytes)
print(f"k_hex = {k_bytes.hex()}")

# Verify HNxorg
xorNG = srp.HNxorg(hash_class, N, g)
print(f"xorNG = {xorNG.hex()}")
print(f"xorNG_len = {len(xorNG)}")

# Verify hN, hg separately
bin_N = srp.long_to_bytes(N)
bin_g = srp.long_to_bytes(g)
padding = len(bin_N) - len(bin_g)
hN = hash_class(bin_N).digest()
hg = hash_class(b'\0' * padding + bin_g).digest()
print(f"hN = {hN.hex()}")
print(f"hg = {hg.hex()}")

# Verify gen_x step by step
print(f"\n--- gen_x breakdown ---")
# With no_username_in_x, username becomes b''
inner_input = b'' + b':' + derived_password
print(f"inner_input[0] = 0x{inner_input[0]:02x} (should be 0x3a = colon)")
print(f"inner_input_len = {len(inner_input)}")
inner_hash = hash_class(inner_input).digest()
print(f"inner_hash = {inner_hash[:8].hex()}")
# Outer: H(hash_class, salt, inner_hash) — both are bytes, no width, no padding
outer_input = salt + inner_hash
x_bytes = hash_class(outer_input).digest()
print(f"x_hex = {x_bytes[:8].hex()}")
print(f"x_input_len = {len(outer_input)} (salt={len(salt)} + inner={len(inner_hash)})")

# Verify with actual gen_x function
x_from_func = srp.gen_x(hash_class, salt, username, derived_password)
x_from_manual = srp.bytes_to_long(x_bytes)
print(f"\ngen_x match: {x_from_func == x_from_manual}")
if x_from_func != x_from_manual:
    print(f"  gen_x result: {srp.long_to_bytes(x_from_func)[:8].hex()}")
    print(f"  manual result: {x_bytes[:8].hex()}")

# Verify calculate_M structure
print(f"\n--- calculate_M breakdown ---")
# Fake A and B for structure verification
A_int = 12345678
B_int = 87654321
K_fake = b'\x00' * 32
I_bytes = username.encode()
hI = hash_class(I_bytes).digest()
print(f"hI = {hI[:8].hex()}")
print(f"hI_len = {len(hI)}")
print(f"long_to_bytes(A) len = {len(srp.long_to_bytes(A_int))}")
print(f"long_to_bytes(B) len = {len(srp.long_to_bytes(B_int))}")

# Verify M uses long_to_bytes (minimal), not padded
M_manual = hash_class()
M_manual.update(xorNG)
M_manual.update(hI)
M_manual.update(salt)
M_manual.update(srp.long_to_bytes(A_int))
M_manual.update(srp.long_to_bytes(B_int))
M_manual.update(K_fake)
M_from_manual = M_manual.digest()

M_from_func = srp.calculate_M(hash_class, N, g, username, salt, A_int, B_int, K_fake)
print(f"\ncalculate_M match: {M_from_func == M_from_manual}")

print("\n✓ All verifications complete")
