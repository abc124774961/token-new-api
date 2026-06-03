import { spawn } from 'node:child_process';
import net from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const desktopDir = path.resolve(__dirname, '..');
const repoRoot = path.resolve(desktopDir, '../..');
const mode = process.argv[2] === 'pro' ? 'pro' : 'dev';
const envFile = path.join(repoRoot, `.env.${mode}`);
const host = '127.0.0.1';
const port = await findOpenPort(host, Number(process.env.AUTOMATION_DESKTOP_DEV_PORT || 5174));
const rendererUrl = `http://${host}:${port}`;
const children = new Set();
let shuttingDown = false;

const env = {
  ...process.env,
  AUTOMATION_ENV_FILE: envFile,
  AUTOMATION_DESKTOP_RENDERER_URL: rendererUrl,
};
const viteBin = localBin('vite');
const electronBin = localBin('electron');

console.log(`[desktop] mode=${mode} env=${envFile}`);
console.log(`[desktop] renderer=${rendererUrl}`);

const vite = spawn(viteBin, ['--host', host, '--port', String(port), '--strictPort'], {
  cwd: desktopDir,
  env,
  stdio: 'inherit',
});
children.add(vite);
vite.on('error', (error) => {
  console.error(`[desktop] failed to start vite: ${error.message}`);
  shutdown(1);
});
vite.on('exit', (code, signal) => {
  children.delete(vite);
  if (!shuttingDown && code !== 0) {
    console.error(`[desktop] vite exited: code=${code} signal=${signal || ''}`);
    shutdown(code || 1);
  }
});

await waitForHTTP(rendererUrl, 30_000);

const electron = spawn(electronBin, ['.'], {
  cwd: desktopDir,
  env,
  stdio: 'inherit',
});
children.add(electron);
electron.on('error', (error) => {
  console.error(`[desktop] failed to start electron: ${error.message}`);
  shutdown(1);
});
electron.on('exit', (code, signal) => {
  children.delete(electron);
  if (!shuttingDown) {
    shutdown(code || (signal ? 1 : 0));
  }
});

process.on('SIGINT', () => shutdown(130));
process.on('SIGTERM', () => shutdown(143));

function shutdown(code = 0) {
  if (shuttingDown) {
    return;
  }
  shuttingDown = true;
  for (const child of children) {
    child.kill('SIGTERM');
  }
  setTimeout(() => process.exit(code), 250).unref();
}

async function findOpenPort(host, startPort) {
  for (let candidate = startPort; candidate < startPort + 100; candidate += 1) {
    if (await isPortOpen(host, candidate)) {
      return candidate;
    }
  }
  throw new Error(`No open desktop dev port found from ${startPort}`);
}

function isPortOpen(host, port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once('error', () => resolve(false));
    server.once('listening', () => {
      server.close(() => resolve(true));
    });
    server.listen(port, host);
  });
}

function localBin(command) {
  return path.join(desktopDir, 'node_modules', '.bin', process.platform === 'win32' ? `${command}.cmd` : command);
}

async function waitForHTTP(url, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        return;
      }
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`Timed out waiting for ${url}: ${lastError?.message || 'not ready'}`);
}
