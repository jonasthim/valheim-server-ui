using System.Collections.Generic;
using System.Globalization;
using System.Text;

namespace ValheimUI.Agent
{
    /// <summary>
    /// Minimal streaming JSON writer. The game ships no JSON library we can
    /// rely on across versions, and the agent only emits flat, known shapes.
    /// </summary>
    internal sealed class JsonWriter
    {
        private readonly StringBuilder _sb = new StringBuilder(2048);
        // One entry per open container: true while it has no members yet.
        private readonly Stack<bool> _empty = new Stack<bool>();
        private bool _afterName;

        public override string ToString() => _sb.ToString();

        private void BeforeValue()
        {
            if (_afterName) { _afterName = false; return; }
            if (_empty.Count == 0) return;
            if (_empty.Peek()) { _empty.Pop(); _empty.Push(false); }
            else _sb.Append(',');
        }

        public JsonWriter BeginObject() { BeforeValue(); _sb.Append('{'); _empty.Push(true); return this; }
        public JsonWriter EndObject() { _sb.Append('}'); _empty.Pop(); return this; }
        public JsonWriter BeginArray() { BeforeValue(); _sb.Append('['); _empty.Push(true); return this; }
        public JsonWriter EndArray() { _sb.Append(']'); _empty.Pop(); return this; }

        public JsonWriter Name(string name)
        {
            BeforeValue();
            WriteString(name);
            _sb.Append(':');
            _afterName = true;
            return this;
        }

        public JsonWriter Value(string s) { BeforeValue(); if (s == null) _sb.Append("null"); else WriteString(s); return this; }
        public JsonWriter Value(bool b) { BeforeValue(); _sb.Append(b ? "true" : "false"); return this; }
        public JsonWriter Value(int n) { BeforeValue(); _sb.Append(n.ToString(CultureInfo.InvariantCulture)); return this; }
        public JsonWriter Value(long n) { BeforeValue(); _sb.Append(n.ToString(CultureInfo.InvariantCulture)); return this; }
        public JsonWriter Value(double d)
        {
            BeforeValue();
            if (double.IsNaN(d) || double.IsInfinity(d)) _sb.Append("null");
            else _sb.Append(d.ToString("R", CultureInfo.InvariantCulture));
            return this;
        }
        public JsonWriter Null() { BeforeValue(); _sb.Append("null"); return this; }
        /// <summary>Writes already-serialised JSON as the next value.</summary>
        public JsonWriter Raw(string json) { BeforeValue(); _sb.Append(json); return this; }

        public JsonWriter Prop(string name, string v) => Name(name).Value(v);
        public JsonWriter Prop(string name, bool v) => Name(name).Value(v);
        public JsonWriter Prop(string name, int v) => Name(name).Value(v);
        public JsonWriter Prop(string name, long v) => Name(name).Value(v);
        public JsonWriter Prop(string name, double v) => Name(name).Value(v);

        private void WriteString(string s)
        {
            _sb.Append('"');
            foreach (var c in s)
            {
                switch (c)
                {
                    case '"': _sb.Append("\\\""); break;
                    case '\\': _sb.Append("\\\\"); break;
                    case '\n': _sb.Append("\\n"); break;
                    case '\r': _sb.Append("\\r"); break;
                    case '\t': _sb.Append("\\t"); break;
                    default:
                        if (c < 0x20) _sb.Append("\\u").Append(((int)c).ToString("x4"));
                        else _sb.Append(c);
                        break;
                }
            }
            _sb.Append('"');
        }

        public static string Quote(string s)
        {
            var w = new JsonWriter();
            w.WriteString(s ?? "");
            return w.ToString();
        }
    }
}
