using System;
using System.IO;
using System.IO.Compression;

namespace ValheimUI.Agent
{
    /// <summary>
    /// Minimal PNG encoder (8-bit RGB, no filtering) so the map can be
    /// produced without Unity's texture APIs, which are unavailable or
    /// unreliable in a headless server. Pure .NET: safe on any thread.
    /// </summary>
    internal static class Png
    {
        private static readonly uint[] CrcTable = BuildCrcTable();

        public static byte[] EncodeRgb(int width, int height, byte[] rgb)
        {
            if (rgb.Length != width * height * 3) throw new ArgumentException("rgb buffer size mismatch");
            return Encode(width, height, rgb, 3, 2);
        }

        /// <summary>8-bit RGBA (colour type 6), four bytes per pixel.</summary>
        public static byte[] EncodeRgba(int width, int height, byte[] rgba)
        {
            if (rgba.Length != width * height * 4) throw new ArgumentException("rgba buffer size mismatch");
            return Encode(width, height, rgba, 4, 6);
        }

        /// <summary>8-bit greyscale (colour type 0), one byte per pixel.</summary>
        public static byte[] EncodeGray(int width, int height, byte[] gray)
        {
            if (gray.Length != width * height) throw new ArgumentException("gray buffer size mismatch");
            return Encode(width, height, gray, 1, 0);
        }

        /// <summary>8-bit greyscale with alpha (colour type 4), two bytes per pixel.</summary>
        public static byte[] EncodeGrayAlpha(int width, int height, byte[] grayAlpha)
        {
            if (grayAlpha.Length != width * height * 2) throw new ArgumentException("gray+alpha buffer size mismatch");
            return Encode(width, height, grayAlpha, 2, 4);
        }

        private static byte[] Encode(int width, int height, byte[] pixels, int channels, byte colourType)
        {
            // Rows are streamed into the deflater with their filter byte and
            // the Adler-32 kept incrementally, so no filtered copy of the
            // whole image is made (64 MB for a 4096² RGBA layer image).
            int stride = width * channels;
            uint adlerA = 1, adlerB = 0;
            var filter = new byte[1];
            byte[] deflated;
            using (var ms = new MemoryStream())
            {
                using (var ds = new DeflateStream(ms, CompressionMode.Compress, true))
                {
                    for (int y = 0; y < height; y++)
                    {
                        ds.Write(filter, 0, 1); // filter: none
                        Adler32Update(ref adlerA, ref adlerB, filter, 0, 1);
                        ds.Write(pixels, y * stride, stride);
                        Adler32Update(ref adlerA, ref adlerB, pixels, y * stride, stride);
                    }
                }
                deflated = ms.ToArray();
            }

            using (var outMs = new MemoryStream(deflated.Length + 1024))
            {
                outMs.Write(new byte[] { 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A }, 0, 8);

                var ihdr = new byte[13];
                WriteBE(ihdr, 0, (uint)width);
                WriteBE(ihdr, 4, (uint)height);
                ihdr[8] = 8;  // bit depth
                ihdr[9] = colourType; // 2 truecolour, 0 greyscale
                ihdr[10] = 0; // compression
                ihdr[11] = 0; // filter
                ihdr[12] = 0; // interlace
                WriteChunk(outMs, "IHDR", ihdr);

                // zlib wrapper around the raw deflate stream.
                var idat = new byte[deflated.Length + 6];
                idat[0] = 0x78;
                idat[1] = 0x9C;
                Buffer.BlockCopy(deflated, 0, idat, 2, deflated.Length);
                WriteBE(idat, deflated.Length + 2, (adlerB << 16) | adlerA);
                WriteChunk(outMs, "IDAT", idat);

                WriteChunk(outMs, "IEND", new byte[0]);
                return outMs.ToArray();
            }
        }

        private static void WriteChunk(Stream s, string type, byte[] data)
        {
            var len = new byte[4];
            WriteBE(len, 0, (uint)data.Length);
            s.Write(len, 0, 4);
            var typeBytes = new byte[] { (byte)type[0], (byte)type[1], (byte)type[2], (byte)type[3] };
            s.Write(typeBytes, 0, 4);
            s.Write(data, 0, data.Length);
            uint crc = Crc32(typeBytes, 0, 4, 0xFFFFFFFF);
            crc = Crc32(data, 0, data.Length, crc) ^ 0xFFFFFFFF;
            var crcBytes = new byte[4];
            WriteBE(crcBytes, 0, crc);
            s.Write(crcBytes, 0, 4);
        }

        private static void WriteBE(byte[] b, int off, uint v)
        {
            b[off] = (byte)(v >> 24);
            b[off + 1] = (byte)(v >> 16);
            b[off + 2] = (byte)(v >> 8);
            b[off + 3] = (byte)v;
        }

        private static uint[] BuildCrcTable()
        {
            var table = new uint[256];
            for (uint n = 0; n < 256; n++)
            {
                uint c = n;
                for (int k = 0; k < 8; k++) c = (c & 1) != 0 ? 0xEDB88320 ^ (c >> 1) : c >> 1;
                table[n] = c;
            }
            return table;
        }

        private static uint Crc32(byte[] buf, int off, int len, uint crc)
        {
            for (int i = off; i < off + len; i++) crc = CrcTable[(crc ^ buf[i]) & 0xFF] ^ (crc >> 8);
            return crc;
        }

        private static void Adler32Update(ref uint a, ref uint b, byte[] data, int off, int len)
        {
            int i = off, end = off + len;
            while (i < end)
            {
                int n = Math.Min(5552, end - i);
                for (; n > 0; n--, i++)
                {
                    a += data[i];
                    b += a;
                }
                a %= 65521;
                b %= 65521;
            }
        }

        private static uint Adler32(byte[] data)
        {
            uint a = 1, b = 0;
            int i = 0;
            while (i < data.Length)
            {
                int n = Math.Min(5552, data.Length - i);
                for (int k = 0; k < n; k++)
                {
                    a += data[i + k];
                    b += a;
                }
                a %= 65521;
                b %= 65521;
                i += n;
            }
            return (b << 16) | a;
        }
    }
}
