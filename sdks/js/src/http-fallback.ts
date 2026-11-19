// Event delivery: fetch where it exists, the Node http module otherwise.

function sendWithHttp(
  url: string,
  headers: Record<string, string>,
  body: string
): Promise<void> {
  return new Promise((resolve, reject) => {
    try {
      // eslint-disable-next-line @typescript-eslint/no-var-requires
      const protocol = url.startsWith("https") ? require("https") : require("http");
      const parsed = new URL(url);
      const req = protocol.request(
        {
          hostname: parsed.hostname,
          port: parsed.port,
          path: parsed.pathname,
          method: "POST",
          headers,
        },
        (res: { statusCode: number }) => {
          if (res.statusCode && res.statusCode >= 400) {
            reject(new Error(`funnelbarn: server returned ${res.statusCode}`));
          } else {
            resolve();
          }
        }
      );
      req.on("error", reject);
      req.write(body);
      req.end();
    } catch (err) {
      reject(err);
    }
  });
}

export async function postJSON(url: string, headers: Record<string, string>, body: string): Promise<void> {
  if (typeof fetch !== "undefined") {
    // Use fetch (browser + Node 18+).
    const response = await fetch(url, { method: "POST", headers, body, keepalive: true });
    if (!response.ok) {
      throw new Error(`funnelbarn: server returned ${response.status}`);
    }
  } else {
    // Node < 18: use http/https module.
    await sendWithHttp(url, headers, body);
  }
}
