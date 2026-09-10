using System;
using System.Collections.Generic;
using System.IO;
using System.Net;
using System.Text;
using System.Threading;
using BepInEx.Logging;

namespace ValheimUI.Agent
{
    /// <summary>
    /// The loopback HTTP API the manager talks to. Runs on its own thread;
    /// never touches game objects directly. Everything but /v1/health needs
    /// the bearer token the manager wrote into the plugin config.
    /// </summary>
    internal sealed class HttpApi
    {
        private const int MaxBodyBytes = 4096;
        private const int CommandTimeoutMs = 5000;

        private readonly ManualLogSource _log;
        private readonly Func<string> _status;
        private readonly Func<long, string> _events;
        private readonly Action<PendingCommand> _enqueue;
        private readonly Func<string> _token;

        private HttpListener _listener;
        private Thread _thread;
        private volatile bool _running;

        public HttpApi(ManualLogSource log, Func<string> status, Func<long, string> events, Action<PendingCommand> enqueue, Func<string> token)
        {
            _log = log;
            _status = status;
            _events = events;
            _enqueue = enqueue;
            _token = token;
        }

        public void Start(string bind, int port)
        {
            _listener = new HttpListener();
            _listener.Prefixes.Add("http://" + bind + ":" + port + "/");
            _listener.Start();
            _running = true;
            _thread = new Thread(Loop) { IsBackground = true, Name = "valheim-ui-agent-http" };
            _thread.Start();
            _log.LogInfo("agent listening on http://" + bind + ":" + port + "/");
        }

        public void Stop()
        {
            _running = false;
            try { _listener?.Stop(); _listener?.Close(); } catch (Exception) { }
        }

        private void Loop()
        {
            while (_running)
            {
                HttpListenerContext ctx;
                try { ctx = _listener.GetContext(); }
                catch (Exception) { if (_running) Thread.Sleep(50); continue; }
                ThreadPool.QueueUserWorkItem(_ => Handle(ctx));
            }
        }

        private void Handle(HttpListenerContext ctx)
        {
            try
            {
                var req = ctx.Request;
                var path = req.Url.AbsolutePath.TrimEnd('/');
                if (req.HttpMethod == "GET" && path == "/v1/health")
                {
                    Json(ctx, 200, "{\"ok\":true,\"agent_version\":" + JsonWriter.Quote(BuildInfo.Version) + "}");
                    return;
                }
                if (!Authorized(req))
                {
                    Json(ctx, 401, "{\"ok\":false,\"error\":\"unauthorized\"}");
                    return;
                }
                if (req.HttpMethod == "GET" && path == "/v1/status")
                {
                    Json(ctx, 200, _status());
                    return;
                }
                if (req.HttpMethod == "GET" && path == "/v1/events")
                {
                    long since = 0;
                    long.TryParse(req.QueryString["since"] ?? "0", out since);
                    Json(ctx, 200, _events(since));
                    return;
                }
                if (req.HttpMethod == "POST" && path.StartsWith("/v1/commands/", StringComparison.Ordinal))
                {
                    var name = path.Substring("/v1/commands/".Length);
                    if (Array.IndexOf(Commands.Names, name) < 0)
                    {
                        Json(ctx, 404, "{\"ok\":false,\"error\":\"unknown command\"}");
                        return;
                    }
                    var cmd = new PendingCommand { Name = name, Args = new Dictionary<string, string>(), Body = ReadBody(req) };
                    foreach (var key in req.QueryString.AllKeys)
                    {
                        if (key != null) cmd.Args[key] = req.QueryString[key] ?? "";
                    }
                    _enqueue(cmd);
                    if (!cmd.Done.Wait(CommandTimeoutMs))
                    {
                        Json(ctx, 504, "{\"ok\":false,\"error\":\"the server did not process the command in time\"}");
                        return;
                    }
                    var r = cmd.Result ?? new CommandResult { Ok = false, Message = "no result" };
                    Json(ctx, r.Ok ? 200 : 400, "{\"ok\":" + (r.Ok ? "true" : "false") + ",\"message\":" + JsonWriter.Quote(r.Message) + "}");
                    return;
                }
                Json(ctx, 404, "{\"ok\":false,\"error\":\"not found\"}");
            }
            catch (Exception e)
            {
                _log.LogWarning("agent request failed: " + e.Message);
                try { Json(ctx, 500, "{\"ok\":false,\"error\":\"internal error\"}"); } catch (Exception) { }
            }
        }

        private bool Authorized(HttpListenerRequest req)
        {
            var want = _token();
            if (string.IsNullOrEmpty(want)) return false;
            var header = req.Headers["Authorization"] ?? "";
            const string prefix = "Bearer ";
            if (!header.StartsWith(prefix, StringComparison.Ordinal)) return false;
            var got = header.Substring(prefix.Length).Trim();
            return ConstantTimeEquals(got, want);
        }

        private static bool ConstantTimeEquals(string a, string b)
        {
            var x = Encoding.UTF8.GetBytes(a);
            var y = Encoding.UTF8.GetBytes(b);
            int diff = x.Length ^ y.Length;
            for (int i = 0; i < x.Length && i < y.Length; i++) diff |= x[i] ^ y[i];
            return diff == 0;
        }

        private static string ReadBody(HttpListenerRequest req)
        {
            if (!req.HasEntityBody) return "";
            if (req.ContentLength64 > MaxBodyBytes) return "";
            using (var ms = new MemoryStream())
            {
                var buf = new byte[1024];
                int total = 0, n;
                while ((n = req.InputStream.Read(buf, 0, buf.Length)) > 0)
                {
                    total += n;
                    if (total > MaxBodyBytes) break;
                    ms.Write(buf, 0, n);
                }
                return Encoding.UTF8.GetString(ms.ToArray());
            }
        }

        private static void Json(HttpListenerContext ctx, int status, string body)
        {
            var bytes = Encoding.UTF8.GetBytes(body);
            var res = ctx.Response;
            res.StatusCode = status;
            res.ContentType = "application/json; charset=utf-8";
            res.ContentLength64 = bytes.Length;
            res.Headers["Cache-Control"] = "no-store";
            res.OutputStream.Write(bytes, 0, bytes.Length);
            res.OutputStream.Close();
        }
    }
}
