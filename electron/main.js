const { app, BrowserWindow, dialog, Tray, Menu, shell, ipcMain, session, WebContentsView, BrowserView } = require('electron');
const { spawn } = require('child_process');
const path = require('path');
const http = require('http');
const fs = require('fs');
const crypto = require('crypto');

let mainWindow;
let adminWindow;
let serverProcess;
let tray = null;
let serverErrorLogs = [];
let browserWorkspace = null;
const PORT = parsePortEnv(process.env.NEW_API_DESKTOP_PORT || process.env.PORT, 3000);
const DEV_FRONTEND_PORT = parsePortEnv(process.env.NEW_API_DESKTOP_FRONTEND_PORT || process.env.DEV_FRONTEND_PORT, 5173);
const STARTUP_MAX_RETRIES = parsePositiveIntEnv(process.env.NEW_API_DESKTOP_STARTUP_MAX_RETRIES, 90);
const STARTUP_RETRY_DELAY_MS = parsePositiveIntEnv(process.env.NEW_API_DESKTOP_STARTUP_RETRY_DELAY_MS, 1000);
const MAX_BROWSER_TABS = 16;
const FINGERPRINT_TARGET_READY_TIMEOUT_MS = parsePositiveIntEnv(process.env.NEW_API_FINGERPRINT_TARGET_READY_TIMEOUT_MS, 2500);
const FINGERPRINT_COMMAND_TIMEOUT_MS = parsePositiveIntEnv(process.env.NEW_API_FINGERPRINT_COMMAND_TIMEOUT_MS, 1500);
const CALLBACK_TOKEN = process.env.TOKEN_ACCOUNT_AUTOMATION_CALLBACK_TOKEN || crypto.randomBytes(24).toString('hex');
let browserWorkspaceIPCRegistered = false;
let browserWorkspaceLoginRegistered = false;
const browserWorkspaceProxyAuthByWebContentsId = new Map();
const browserWorkspaceSessionProfiles = new Map();
const browserWorkspaceHeaderHookPartitions = new Set();

app.commandLine.appendSwitch('webrtc-ip-handling-policy', 'disable_non_proxied_udp');
app.commandLine.appendSwitch('force-webrtc-ip-handling-policy', 'disable_non_proxied_udp');

function parsePortEnv(value, fallback) {
  const parsed = Number.parseInt(String(value || ''), 10);
  if (!Number.isFinite(parsed) || parsed <= 0 || parsed > 65535) {
    return fallback;
  }
  return parsed;
}

function parsePositiveIntEnv(value, fallback) {
  const parsed = Number.parseInt(String(value || ''), 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}

function withTimeout(promise, timeoutMS, label) {
  let timer;
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error(`${label} timed out after ${timeoutMS}ms`)), timeoutMS);
  });
  return Promise.race([
    Promise.resolve(promise).finally(() => clearTimeout(timer)),
    timeout
  ]);
}

// 保存日志到文件并打开
function saveAndOpenErrorLog() {
  try {
    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
    const logFileName = `new-api-crash-${timestamp}.log`;
    const logDir = app.getPath('logs');
    const logFilePath = path.join(logDir, logFileName);
    
    // 确保日志目录存在
    if (!fs.existsSync(logDir)) {
      fs.mkdirSync(logDir, { recursive: true });
    }
    
    // 写入日志
    const logContent = `New API 崩溃日志
生成时间: ${new Date().toLocaleString('zh-CN')}
平台: ${process.platform}
架构: ${process.arch}
应用版本: ${app.getVersion()}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

完整错误日志:

${serverErrorLogs.join('\n')}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

日志文件位置: ${logFilePath}
`;
    
    fs.writeFileSync(logFilePath, logContent, 'utf8');
    
    // 打开日志文件
    shell.openPath(logFilePath).then((error) => {
      if (error) {
        console.error('Failed to open log file:', error);
        // 如果打开文件失败，至少显示文件位置
        shell.showItemInFolder(logFilePath);
      }
    });
    
    return logFilePath;
  } catch (err) {
    console.error('Failed to save error log:', err);
    return null;
  }
}

// 分析错误日志，识别常见错误并提供解决方案
function analyzeError(errorLogs) {
  const allLogs = errorLogs.join('\n');
  
  // 检测端口占用错误
  if (allLogs.includes('failed to start HTTP server') || 
      allLogs.includes('bind: address already in use') ||
      allLogs.includes('listen tcp') && allLogs.includes('bind: address already in use')) {
    return {
      type: '端口被占用',
      title: '端口 ' + PORT + ' 被占用',
      message: '无法启动服务器，端口已被其他程序占用',
      solution: `可能的解决方案：\n\n1. 关闭占用端口 ${PORT} 的其他程序\n2. 检查是否已经运行了另一个 New API 实例\n3. 使用以下命令查找占用端口的进程：\n   Mac/Linux: lsof -i :${PORT}\n   Windows: netstat -ano | findstr :${PORT}\n4. 重启电脑以释放端口`
    };
  }
  
  // 检测数据库错误
  if (allLogs.includes('database is locked') || 
      allLogs.includes('unable to open database')) {
    return {
      type: '数据文件被占用',
      title: '无法访问数据文件',
      message: '应用的数据文件正被其他程序占用',
      solution: '可能的解决方案：\n\n1. 检查是否已经打开了另一个 New API 窗口\n   - 查看任务栏/Dock 中是否有其他 New API 图标\n   - 查看系统托盘（Windows）或菜单栏（Mac）中是否有 New API 图标\n\n2. 如果刚刚关闭过应用，请等待 10 秒后再试\n\n3. 重启电脑以释放被占用的文件\n\n4. 如果问题持续，可以尝试：\n   - 退出所有 New API 实例\n   - 删除数据目录中的临时文件（.db-shm 和 .db-wal）\n   - 重新启动应用'
    };
  }
  
  // 检测权限错误
  if (allLogs.includes('permission denied') || 
      allLogs.includes('access denied')) {
    return {
      type: '权限错误',
      title: '权限不足',
      message: '程序没有足够的权限执行操作',
      solution: '可能的解决方案：\n\n1. 以管理员/root权限运行程序\n2. 检查数据目录的读写权限\n3. 检查可执行文件的权限\n4. 在 Mac 上，检查安全性与隐私设置'
    };
  }
  
  // 检测网络错误
  if (allLogs.includes('network is unreachable') || 
      allLogs.includes('no such host') ||
      allLogs.includes('connection refused')) {
    return {
      type: '网络错误',
      title: '网络连接失败',
      message: '无法建立网络连接',
      solution: '可能的解决方案：\n\n1. 检查网络连接是否正常\n2. 检查防火墙设置\n3. 检查代理配置\n4. 确认目标服务器地址正确'
    };
  }
  
  // 检测配置文件错误
  if (allLogs.includes('invalid configuration') || 
      allLogs.includes('failed to parse config') ||
      allLogs.includes('yaml') || allLogs.includes('json') && allLogs.includes('parse')) {
    return {
      type: '配置错误',
      title: '配置文件错误',
      message: '配置文件格式不正确或包含无效配置',
      solution: '可能的解决方案：\n\n1. 检查配置文件格式是否正确\n2. 恢复默认配置\n3. 删除配置文件让程序重新生成\n4. 查看文档了解正确的配置格式'
    };
  }
  
  // 检测内存不足
  if (allLogs.includes('out of memory') || 
      allLogs.includes('cannot allocate memory')) {
    return {
      type: '内存不足',
      title: '系统内存不足',
      message: '程序运行时内存不足',
      solution: '可能的解决方案：\n\n1. 关闭其他占用内存的程序\n2. 增加系统可用内存\n3. 重启电脑释放内存\n4. 检查是否存在内存泄漏'
    };
  }
  
  // 检测文件不存在错误
  if (allLogs.includes('no such file or directory') || 
      allLogs.includes('cannot find the file')) {
    return {
      type: '文件缺失',
      title: '找不到必需的文件',
      message: '缺少程序运行所需的文件',
      solution: '可能的解决方案：\n\n1. 重新安装应用程序\n2. 检查安装目录是否完整\n3. 确保所有依赖文件都存在\n4. 检查文件路径是否正确'
    };
  }
  
  return null;
}

function getBinaryPath() {
  const isDev = process.env.NODE_ENV === 'development';
  const platform = process.platform;

  if (isDev) {
    const binaryName = platform === 'win32' ? 'new-api.exe' : 'new-api';
    return path.join(__dirname, '..', binaryName);
  }

  let binaryName;
  switch (platform) {
    case 'win32':
      binaryName = 'new-api.exe';
      break;
    case 'darwin':
      binaryName = 'new-api';
      break;
    case 'linux':
      binaryName = 'new-api';
      break;
    default:
      binaryName = 'new-api';
  }

  return path.join(process.resourcesPath, 'bin', binaryName);
}

// Check if a server is available with retry logic
function checkServerAvailability(port, maxRetries = 30, retryDelay = 1000) {
  return new Promise((resolve, reject) => {
    let currentAttempt = 0;
    
    const tryConnect = () => {
      currentAttempt++;
      
      if (currentAttempt % 5 === 1 && currentAttempt > 1) {
        console.log(`Attempting to connect to port ${port}... (attempt ${currentAttempt}/${maxRetries})`);
      }
      
      const req = http.get({
        hostname: '127.0.0.1', // Use IPv4 explicitly instead of 'localhost' to avoid IPv6 issues
        port: port,
        timeout: 10000
      }, (res) => {
        // Server responded, connection successful
        req.destroy();
        console.log(`✓ Successfully connected to port ${port} (status: ${res.statusCode})`);
        resolve();
      });

      req.on('error', (err) => {
        if (currentAttempt >= maxRetries) {
          reject(new Error(`Failed to connect to port ${port} after ${maxRetries} attempts: ${err.message}`));
        } else {
          setTimeout(tryConnect, retryDelay);
        }
      });

      req.on('timeout', () => {
        req.destroy();
        if (currentAttempt >= maxRetries) {
          reject(new Error(`Connection timeout on port ${port} after ${maxRetries} attempts`));
        } else {
          setTimeout(tryConnect, retryDelay);
        }
      });
    };
    
    tryConnect();
  });
}

function startServer() {
  return new Promise((resolve, reject) => {
    const isDev = process.env.NODE_ENV === 'development';

    const userDataPath = app.getPath('userData');
    const dataDir = path.join(userDataPath, 'data');
    
    // 设置环境变量供 preload.js 使用
    process.env.ELECTRON_DATA_DIR = dataDir;
    
    if (isDev) {
      // 开发模式：假设开发者手动启动了 Go 后端和前端开发服务器
      // 只需要等待前端开发服务器就绪
      console.log('Development mode: skipping server startup');
      console.log('Please make sure you have started:');
      console.log(`  1. Go backend: TOKEN_ACCOUNT_AUTOMATION_CALLBACK_TOKEN=${CALLBACK_TOKEN} PORT=${PORT} go run main.go`);
      console.log(`  2. Frontend dev server: cd web/classic && bun run dev (port ${DEV_FRONTEND_PORT})`);
      console.log('');
      console.log('Checking if servers are running...');
      
      // First check if both servers are accessible
      Promise.all([
        checkServerAvailability(PORT, STARTUP_MAX_RETRIES, STARTUP_RETRY_DELAY_MS),
        checkServerAvailability(DEV_FRONTEND_PORT, STARTUP_MAX_RETRIES, STARTUP_RETRY_DELAY_MS)
      ])
        .then(() => {
          console.log(`✓ Backend server is accessible on port ${PORT}`);
          console.log(`✓ Frontend dev server is accessible on port ${DEV_FRONTEND_PORT}`);
          resolve();
        })
        .catch((err) => {
          console.error(`✗ Cannot connect to required dev servers on ports ${PORT} and ${DEV_FRONTEND_PORT}`);
          console.error('Please make sure both dev servers are running with the callback token shown above.');
          reject(err);
        });
      return;
    }

    // 生产模式：启动二进制服务器
    const env = {
      ...process.env,
      PORT: PORT.toString(),
      TOKEN_ACCOUNT_AUTOMATION_CALLBACK_TOKEN: CALLBACK_TOKEN
    };

    if (!fs.existsSync(dataDir)) {
      fs.mkdirSync(dataDir, { recursive: true });
    }

    env.SQLITE_PATH = path.join(dataDir, 'new-api.db');
    
    console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
    console.log('📁 您的数据存储位置：');
    console.log('   ' + dataDir);
    console.log('   💡 备份提示：复制此目录即可备份所有数据');
    console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');

    const binaryPath = getBinaryPath();
    const workingDir = process.resourcesPath;
    
    console.log('Starting server from:', binaryPath);

    serverProcess = spawn(binaryPath, [], {
      env,
      cwd: workingDir
    });

    serverProcess.stdout.on('data', (data) => {
      console.log(`Server: ${data}`);
    });

    serverProcess.stderr.on('data', (data) => {
      const errorMsg = data.toString();
      console.error(`Server Error: ${errorMsg}`);
      serverErrorLogs.push(errorMsg);
      // 只保留最近的100条错误日志
      if (serverErrorLogs.length > 100) {
        serverErrorLogs.shift();
      }
    });

    serverProcess.on('error', (err) => {
      console.error('Failed to start server:', err);
      reject(err);
    });

    serverProcess.on('close', (code) => {
      console.log(`Server process exited with code ${code}`);
      
      // 如果退出代码不是0，说明服务器异常退出
      if (code !== 0 && code !== null) {
        const errorDetails = serverErrorLogs.length > 0 
          ? serverErrorLogs.slice(-20).join('\n') 
          : '没有捕获到错误日志';
        
        // 分析错误类型
        const knownError = analyzeError(serverErrorLogs);
        
        let dialogOptions;
        if (knownError) {
          // 识别到已知错误，显示友好的错误信息和解决方案
          dialogOptions = {
            type: 'error',
            title: knownError.title,
            message: knownError.message,
            detail: `${knownError.solution}\n\n━━━━━━━━━━━━━━━━━━━━━━\n\n退出代码: ${code}\n\n错误类型: ${knownError.type}\n\n最近的错误日志:\n${errorDetails}`,
            buttons: ['退出应用', '查看完整日志'],
            defaultId: 0,
            cancelId: 0
          };
        } else {
          // 未识别的错误，显示通用错误信息
          dialogOptions = {
            type: 'error',
            title: '服务器崩溃',
            message: '服务器进程异常退出',
            detail: `退出代码: ${code}\n\n最近的错误信息:\n${errorDetails}`,
            buttons: ['退出应用', '查看完整日志'],
            defaultId: 0,
            cancelId: 0
          };
        }
        
        dialog.showMessageBox(dialogOptions).then((result) => {
          if (result.response === 1) {
            // 用户选择查看详情，保存并打开日志文件
            const logPath = saveAndOpenErrorLog();
            
            // 显示确认对话框
            const confirmMessage = logPath 
              ? `日志已保存到:\n${logPath}\n\n日志文件已在默认文本编辑器中打开。\n\n点击"退出"关闭应用程序。`
              : '日志保存失败，但已在控制台输出。\n\n点击"退出"关闭应用程序。';
            
            dialog.showMessageBox({
              type: 'info',
              title: '日志已保存',
              message: confirmMessage,
              buttons: ['退出'],
              defaultId: 0
            }).then(() => {
              app.isQuitting = true;
              app.quit();
            });
            
            // 同时在控制台输出
            console.log('=== 完整错误日志 ===');
            console.log(serverErrorLogs.join('\n'));
          } else {
            // 用户选择直接退出
            app.isQuitting = true;
            app.quit();
          }
        });
      } else {
        // 正常退出（code为0或null），直接关闭窗口
        if (mainWindow && !mainWindow.isDestroyed()) {
          mainWindow.close();
        }
      }
    });

    checkServerAvailability(PORT, STARTUP_MAX_RETRIES, STARTUP_RETRY_DELAY_MS)
      .then(() => {
        console.log(`✓ Backend server is accessible on port ${PORT}`);
        resolve();
      })
      .catch((err) => {
        console.error('✗ Failed to connect to backend server');
        reject(err);
      });
  });
}

function backendPort() {
  return PORT;
}

function requestBackendJSON(pathname, token = CALLBACK_TOKEN) {
  return new Promise((resolve, reject) => {
    const req = http.get({
      hostname: '127.0.0.1',
      port: backendPort(),
      path: pathname,
      timeout: 15000,
      headers: {
        Authorization: `Bearer ${token}`
      }
    }, (res) => {
      const chunks = [];
      res.on('data', (chunk) => chunks.push(chunk));
      res.on('end', () => {
        const body = Buffer.concat(chunks).toString('utf8');
        let payload;
        try {
          payload = JSON.parse(body);
        } catch (err) {
          reject(new Error(`Invalid backend JSON: ${err.message}`));
          return;
        }
        if (res.statusCode < 200 || res.statusCode >= 300 || payload.success === false) {
          reject(new Error(payload.message || `Backend request failed with status ${res.statusCode}`));
          return;
        }
        resolve(payload.data);
      });
    });
    req.on('error', reject);
    req.on('timeout', () => {
      req.destroy();
      reject(new Error('Backend request timed out'));
    });
  });
}

function loadJSONFile(filePath, fallback) {
  try {
    if (!fs.existsSync(filePath)) {
      return fallback;
    }
    return JSON.parse(fs.readFileSync(filePath, 'utf8'));
  } catch (err) {
    console.warn('Failed to read JSON file:', filePath, err);
    return fallback;
  }
}

function saveJSONFile(filePath, value) {
  const dir = path.dirname(filePath);
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }
  fs.writeFileSync(filePath, JSON.stringify(value, null, 2), 'utf8');
}

function hashNumber(value) {
  const digest = crypto.createHash('sha256').update(String(value)).digest();
  return digest.readUInt32BE(0);
}

function stableHash(seed, key) {
  return hashNumber(`${seed}:${key}`);
}

function pickStable(values, seed) {
  return values[Math.abs(seed) % values.length];
}

function pickStableByKey(values, seed, key) {
  return pickStable(values, stableHash(seed, key));
}

function stableInt(seed, key, min, max) {
  const lower = Math.ceil(min);
  const upper = Math.floor(max);
  return lower + (stableHash(seed, key) % (upper - lower + 1));
}

function stableFloat(seed, key, min, max, precision = 6) {
  const ratio = stableHash(seed, key) / 0xffffffff;
  return Number((min + (max - min) * ratio).toFixed(precision));
}

function browserUserAgent(seed) {
  const chromeVersion = pickStable(['126.0.0.0', '127.0.0.0', '128.0.0.0'], seed);
  if (process.platform === 'win32') {
    return `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/${chromeVersion} Safari/537.36`;
  }
  if (process.platform === 'darwin') {
    return `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/${chromeVersion} Safari/537.36`;
  }
  return `Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/${chromeVersion} Safari/537.36`;
}

function browserPlatformForProcess() {
  if (process.platform === 'darwin') return 'MacIntel';
  if (process.platform === 'win32') return 'Win32';
  return 'Linux x86_64';
}

function screenProfileForAccount(viewport, seed) {
  const presets = process.platform === 'darwin'
    ? [
        { width: 1440, height: 900, pixelRatio: 2 },
        { width: 1512, height: 982, pixelRatio: 2 },
        { width: 1728, height: 1117, pixelRatio: 2 },
        { width: 1920, height: 1080, pixelRatio: 1 }
      ]
    : process.platform === 'win32'
      ? [
          { width: 1366, height: 768, pixelRatio: 1 },
          { width: 1440, height: 900, pixelRatio: 1 },
          { width: 1536, height: 864, pixelRatio: 1.25 },
          { width: 1920, height: 1080, pixelRatio: 1 }
        ]
      : [
          { width: 1366, height: 768, pixelRatio: 1 },
          { width: 1440, height: 900, pixelRatio: 1 },
          { width: 1600, height: 900, pixelRatio: 1 },
          { width: 1920, height: 1080, pixelRatio: 1 }
        ];
  const picked = pickStableByKey(presets, seed, 'screen');
  const width = Math.max(picked.width, viewport.width);
  const height = Math.max(picked.height, viewport.height + 80);
  return {
    width,
    height,
    availWidth: width,
    availHeight: Math.max(viewport.height, height - stableInt(seed, 'screenChromeHeight', 32, 88)),
    colorDepth: 24,
    pixelDepth: 24,
    pixelRatio: picked.pixelRatio
  };
}

function webGLProfileForAccount(seed) {
  const profiles = process.platform === 'darwin'
    ? [
        {
          vendor: 'Google Inc. (Apple)',
          renderer: 'ANGLE (Apple, ANGLE Metal Renderer: Apple M1, Unspecified Version)'
        },
        {
          vendor: 'Google Inc. (Intel Inc.)',
          renderer: 'ANGLE (Intel Inc., Intel(R) Iris(TM) Plus Graphics OpenGL Engine, OpenGL 4.1)'
        }
      ]
    : process.platform === 'win32'
      ? [
          {
            vendor: 'Google Inc. (Intel)',
            renderer: 'ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)'
          },
          {
            vendor: 'Google Inc. (NVIDIA)',
            renderer: 'ANGLE (NVIDIA, NVIDIA GeForce GTX 1660 Direct3D11 vs_5_0 ps_5_0, D3D11)'
          },
          {
            vendor: 'Google Inc. (AMD)',
            renderer: 'ANGLE (AMD, AMD Radeon(TM) Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)'
          }
        ]
      : [
          {
            vendor: 'Google Inc. (Intel)',
            renderer: 'ANGLE (Intel, Mesa Intel(R) UHD Graphics 620, OpenGL 4.6)'
          },
          {
            vendor: 'Google Inc. (AMD)',
            renderer: 'ANGLE (AMD, Radeon RX 580 Series, OpenGL 4.6)'
          }
        ];
  return pickStableByKey(profiles, seed, 'webgl');
}

function connectionProfileForAccount(seed) {
  return {
    effectiveType: pickStableByKey(['4g', '4g', '3g'], seed, 'connectionEffectiveType'),
    downlink: stableFloat(seed, 'connectionDownlink', 8, 72, 1),
    rtt: stableInt(seed, 'connectionRtt', 35, 160),
    saveData: false
  };
}

function languageForAccount(account) {
  const region = String(account?.proxy?.region_code || '').toUpperCase();
  const country = String(account?.proxy?.country_name || '').toLowerCase();
  if (region === 'CN' || country.includes('china')) return 'zh-CN,zh;q=0.9,en;q=0.7';
  if (region === 'JP' || country.includes('japan')) return 'ja-JP,ja;q=0.9,en-US;q=0.7,en;q=0.6';
  if (region === 'KR' || country.includes('korea')) return 'ko-KR,ko;q=0.9,en-US;q=0.7,en;q=0.6';
  if (region === 'TW') return 'zh-TW,zh;q=0.9,en-US;q=0.7,en;q=0.6';
  return 'en-US,en;q=0.9';
}

function buildFingerprintProfile(account, storedProfile) {
  const profileKey = account.profile_key || `channel-${account.channel_id}-credential-${account.credential_index}`;
  const seed = storedProfile?.seed || crypto.createHash('sha256').update(profileKey).digest('hex').slice(0, 16);
  const seedNum = hashNumber(seed);
  const viewport = storedProfile?.viewport || pickStable([
    { width: 1365, height: 768 },
    { width: 1440, height: 900 },
    { width: 1536, height: 864 },
    { width: 1600, height: 900 },
    { width: 1728, height: 972 }
  ], seedNum);
  const screen = storedProfile?.screen || screenProfileForAccount(viewport, seed);
  return {
    fingerprintVersion: 2,
    seed,
    userAgent: storedProfile?.userAgent || browserUserAgent(seedNum),
    language: storedProfile?.language || languageForAccount(account),
    timezone: storedProfile?.timezone || account?.proxy?.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
    viewport,
    screen,
    platform: process.platform,
    browserPlatform: storedProfile?.browserPlatform || browserPlatformForProcess(),
    hardwareConcurrency: storedProfile?.hardwareConcurrency || pickStableByKey([4, 6, 8, 8, 10, 12], seed, 'hardwareConcurrency'),
    deviceMemory: storedProfile?.deviceMemory || pickStableByKey([4, 8, 8, 16], seed, 'deviceMemory'),
    maxTouchPoints: storedProfile?.maxTouchPoints || 0,
    doNotTrack: storedProfile?.doNotTrack || null,
    webgl: storedProfile?.webgl || webGLProfileForAccount(seed),
    canvasNoise: storedProfile?.canvasNoise || stableInt(seed, 'canvasNoise', 1, 9),
    audioNoise: storedProfile?.audioNoise || stableFloat(seed, 'audioNoise', 0.0000002, 0.0000018, 10),
    connection: storedProfile?.connection || connectionProfileForAccount(seed),
    webrtcPolicy: 'disable_non_proxied_udp'
  };
}

function sanitizeAccountForRenderer(account) {
  const cloned = JSON.parse(JSON.stringify(account || {}));
  delete cloned.proxy_rules;
  if (cloned.proxy) {
    delete cloned.proxy.address;
    delete cloned.proxy.username;
  }
  return cloned;
}

function parseProxyRules(proxyRules) {
  if (!proxyRules) {
    return { rules: '', auth: null };
  }
  try {
    const parsed = new URL(proxyRules);
    if (parsed.protocol === 'socks5h:') {
      parsed.protocol = 'socks5:';
    }
    const auth = parsed.username
      ? {
          username: decodeURIComponent(parsed.username),
          password: decodeURIComponent(parsed.password || '')
        }
      : null;
    parsed.username = '';
    parsed.password = '';
    return {
      rules: parsed.toString().replace(/\/$/, ''),
      auth
    };
  } catch (err) {
    return { rules: proxyRules, auth: null };
  }
}

function browserPlatformForProfile(profile) {
  if (profile?.platform === 'darwin') return 'MacIntel';
  if (profile?.platform === 'win32') return 'Win32';
  return 'Linux x86_64';
}

function localeForProfile(profile) {
  const language = String(profile?.language || 'en-US').split(',')[0].split(';')[0].trim();
  return language || 'en-US';
}

function buildFingerprintInjectionScript(profile) {
  const scriptProfile = {
    language: profile.language,
    languages: String(profile.language || 'en-US')
      .split(',')
      .map((item) => item.split(';')[0].trim())
      .filter(Boolean),
    timezone: profile.timezone,
    platform: profile.browserPlatform || browserPlatformForProfile(profile),
    hardwareConcurrency: profile.hardwareConcurrency,
    deviceMemory: profile.deviceMemory,
    maxTouchPoints: profile.maxTouchPoints || 0,
    doNotTrack: profile.doNotTrack,
    screen: profile.screen,
    webgl: profile.webgl,
    canvasNoise: profile.canvasNoise,
    audioNoise: profile.audioNoise,
    connection: profile.connection
  };
  return `
(() => {
  const profile = ${JSON.stringify(scriptProfile)};
  const defineGetter = (target, prop, value) => {
    try {
      Object.defineProperty(target, prop, { get: () => value, configurable: true });
    } catch (_err) {}
  };
  const defineValue = (target, prop, value) => {
    try {
      Object.defineProperty(target, prop, { value, configurable: true, writable: false });
    } catch (_err) {}
  };
  const languages = Array.isArray(profile.languages) && profile.languages.length ? profile.languages : ['en-US', 'en'];
  const navigatorProto = window.Navigator && Navigator.prototype;
  if (navigatorProto) {
    defineGetter(navigatorProto, 'language', languages[0]);
    defineGetter(navigatorProto, 'languages', Object.freeze(languages.slice()));
    defineGetter(navigatorProto, 'platform', profile.platform);
    defineGetter(navigatorProto, 'hardwareConcurrency', profile.hardwareConcurrency);
    defineGetter(navigatorProto, 'deviceMemory', profile.deviceMemory);
    defineGetter(navigatorProto, 'maxTouchPoints', profile.maxTouchPoints || 0);
    defineGetter(navigatorProto, 'webdriver', undefined);
    if (profile.doNotTrack !== null && profile.doNotTrack !== undefined) {
      defineGetter(navigatorProto, 'doNotTrack', String(profile.doNotTrack));
    }
  }

  if (window.Screen && profile.screen) {
    const screenProto = Screen.prototype;
    defineGetter(screenProto, 'width', profile.screen.width);
    defineGetter(screenProto, 'height', profile.screen.height);
    defineGetter(screenProto, 'availWidth', profile.screen.availWidth);
    defineGetter(screenProto, 'availHeight', profile.screen.availHeight);
    defineGetter(screenProto, 'colorDepth', profile.screen.colorDepth || 24);
    defineGetter(screenProto, 'pixelDepth', profile.screen.pixelDepth || 24);
    defineGetter(window, 'devicePixelRatio', profile.screen.pixelRatio || 1);
    defineGetter(window, 'outerWidth', profile.screen.width);
    defineGetter(window, 'outerHeight', profile.screen.height);
  }

  if (window.Intl && Intl.DateTimeFormat && profile.timezone) {
    const NativeDateTimeFormat = Intl.DateTimeFormat;
    const PatchedDateTimeFormat = function(locales, options = {}) {
      return new NativeDateTimeFormat(locales, { timeZone: profile.timezone, ...options });
    };
    PatchedDateTimeFormat.prototype = NativeDateTimeFormat.prototype;
    PatchedDateTimeFormat.supportedLocalesOf = NativeDateTimeFormat.supportedLocalesOf.bind(NativeDateTimeFormat);
    defineValue(Intl, 'DateTimeFormat', PatchedDateTimeFormat);
  }

  const patchWebGL = (Ctor) => {
    if (!Ctor || !Ctor.prototype || !profile.webgl) return;
    const nativeGetParameter = Ctor.prototype.getParameter;
    if (typeof nativeGetParameter !== 'function') return;
    defineValue(Ctor.prototype, 'getParameter', function(parameter) {
      if (parameter === 37445) return profile.webgl.vendor;
      if (parameter === 37446) return profile.webgl.renderer;
      return nativeGetParameter.apply(this, arguments);
    });
  };
  patchWebGL(window.WebGLRenderingContext);
  patchWebGL(window.WebGL2RenderingContext);

  const noiseCanvas = (canvas) => {
    try {
      const width = Math.min(canvas.width || 0, 32);
      const height = Math.min(canvas.height || 0, 32);
      if (!width || !height || canvas.__newApiFingerprintNoised) return;
      const context = canvas.getContext('2d', { willReadFrequently: true });
      if (!context) return;
      const imageData = context.getImageData(0, 0, width, height);
      const shift = Number(profile.canvasNoise || 1);
      for (let i = 0; i < imageData.data.length; i += 16) {
        imageData.data[i] = (imageData.data[i] + shift) & 255;
        imageData.data[i + 1] = (imageData.data[i + 1] + shift + 1) & 255;
        imageData.data[i + 2] = (imageData.data[i + 2] + shift + 2) & 255;
      }
      context.putImageData(imageData, 0, 0);
      defineValue(canvas, '__newApiFingerprintNoised', true);
    } catch (_err) {}
  };
  if (window.HTMLCanvasElement) {
    const nativeToDataURL = HTMLCanvasElement.prototype.toDataURL;
    const nativeToBlob = HTMLCanvasElement.prototype.toBlob;
    defineValue(HTMLCanvasElement.prototype, 'toDataURL', function() {
      noiseCanvas(this);
      return nativeToDataURL.apply(this, arguments);
    });
    if (typeof nativeToBlob === 'function') {
      defineValue(HTMLCanvasElement.prototype, 'toBlob', function() {
        noiseCanvas(this);
        return nativeToBlob.apply(this, arguments);
      });
    }
  }

  if (window.CanvasRenderingContext2D) {
    const nativeGetImageData = CanvasRenderingContext2D.prototype.getImageData;
    if (typeof nativeGetImageData === 'function') {
      defineValue(CanvasRenderingContext2D.prototype, 'getImageData', function() {
        const imageData = nativeGetImageData.apply(this, arguments);
        try {
          const shift = Number(profile.canvasNoise || 1);
          for (let i = 0; i < imageData.data.length; i += 64) {
            imageData.data[i] = (imageData.data[i] + shift) & 255;
          }
        } catch (_err) {}
        return imageData;
      });
    }
  }

  if (window.AudioBuffer && AudioBuffer.prototype) {
    const nativeGetChannelData = AudioBuffer.prototype.getChannelData;
    if (typeof nativeGetChannelData === 'function') {
      defineValue(AudioBuffer.prototype, 'getChannelData', function() {
        const data = nativeGetChannelData.apply(this, arguments);
        try {
          if (!this.__newApiFingerprintAudioNoised) {
            const noise = Number(profile.audioNoise || 0.0000005);
            for (let i = 0; i < data.length; i += 97) {
              data[i] = data[i] + noise;
            }
            defineValue(this, '__newApiFingerprintAudioNoised', true);
          }
        } catch (_err) {}
        return data;
      });
    }
  }

  if (navigator.connection && profile.connection) {
    const connectionProto = Object.getPrototypeOf(navigator.connection);
    if (connectionProto) {
      defineGetter(connectionProto, 'effectiveType', profile.connection.effectiveType || '4g');
      defineGetter(connectionProto, 'downlink', profile.connection.downlink || 10);
      defineGetter(connectionProto, 'rtt', profile.connection.rtt || 80);
      defineGetter(connectionProto, 'saveData', Boolean(profile.connection.saveData));
    }
  }
})();
`;
}

async function applyFingerprintEmulation(webContents, profile) {
  webContents.setUserAgent(profile.userAgent);
  const result = {
    applied: true,
    warnings: []
  };
  if (!webContents.getURL()) {
    try {
      await withTimeout(
        webContents.loadURL('about:blank'),
        FINGERPRINT_TARGET_READY_TIMEOUT_MS,
        'Browser fingerprint target initialization'
      );
    } catch (err) {
      console.warn('Failed to initialize browser fingerprint target:', err.message);
      result.applied = false;
      result.warnings.push(`target initialization: ${err.message}`);
    }
  }
  try {
    if (!webContents.debugger.isAttached()) {
      webContents.debugger.attach('1.3');
    }
  } catch (err) {
    console.warn('Failed to attach debugger for browser fingerprint emulation:', err.message);
    result.applied = false;
    result.warnings.push(err.message);
    return result;
  }
  const commands = [
    ['Network.setUserAgentOverride', {
      userAgent: profile.userAgent,
      acceptLanguage: profile.language,
      platform: profile.browserPlatform || browserPlatformForProfile(profile)
    }],
    ['Page.addScriptToEvaluateOnNewDocument', {
      source: buildFingerprintInjectionScript(profile)
    }],
    profile.timezone
      ? ['Emulation.setTimezoneOverride', { timezoneId: profile.timezone }]
      : null,
    ['Emulation.setLocaleOverride', {
      locale: localeForProfile(profile)
    }]
  ].filter(Boolean);
  for (const [command, params] of commands) {
    try {
      await withTimeout(
        webContents.debugger.sendCommand(command, params),
        FINGERPRINT_COMMAND_TIMEOUT_MS,
        command
      );
    } catch (err) {
      console.warn(`Failed to apply ${command}:`, err.message);
      result.applied = false;
      result.warnings.push(`${command}: ${err.message}`);
    }
  }
  return result;
}

function registerBrowserWorkspaceLoginHandler() {
  if (browserWorkspaceLoginRegistered) {
    return;
  }
  browserWorkspaceLoginRegistered = true;
  app.on('login', (event, webContents, _request, authInfo, callback) => {
    if (!authInfo?.isProxy || !webContents) {
      return;
    }
    const auth = browserWorkspaceProxyAuthByWebContentsId.get(webContents.id);
    if (!auth) {
      return;
    }
    event.preventDefault();
    callback(auth.username, auth.password);
  });
}

function currentBrowserWorkspace() {
  if (!browserWorkspace) {
    throw new Error('浏览器工作台未初始化');
  }
  return browserWorkspace;
}

class BrowserWorkspace {
  constructor(window) {
    this.window = window;
    this.tabs = new Map();
    this.activeTabId = '';
    this.bounds = { x: 0, y: 160, width: 1080, height: 480 };
    this.profilePath = path.join(app.getPath('userData'), 'browser-profiles.json');
    this.profileStore = loadJSONFile(this.profilePath, { profiles: {} });
    this.accountsCache = [];
    this.lastAccountsError = '';
    this.viewClass = WebContentsView || BrowserView;
    this.usingWebContentsView = Boolean(WebContentsView);
  }

  async listAccounts(filters = {}) {
    const params = new URLSearchParams({
      page_size: '500',
      status: filters.status || 'all'
    });
    if (filters.keyword) params.set('keyword', filters.keyword);
    if (filters.channel_id) params.set('channel_id', String(filters.channel_id));
    if (filters.brand) params.set('brand', filters.brand);
    const data = await requestBackendJSON(`/api/internal/electron-browser/accounts?${params.toString()}`);
    this.accountsCache = Array.isArray(data?.items) ? data.items : [];
    this.lastAccountsError = '';
    return {
      ...data,
      items: this.accountsCache.map((account) => sanitizeAccountForRenderer(account))
    };
  }

  async openAccount(payload = {}) {
    const profileKey = String(payload.profileKey || payload.profile_key || '').trim();
    let account = this.accountsCache.find((item) => item.profile_key === profileKey);
    if (!account && payload.channelId !== undefined && payload.credentialIndex !== undefined) {
      account = this.accountsCache.find((item) =>
        Number(item.channel_id) === Number(payload.channelId) &&
        Number(item.credential_index) === Number(payload.credentialIndex)
      );
    }
    if (!account) {
      await this.listAccounts({});
      account = this.accountsCache.find((item) =>
        item.profile_key === profileKey ||
        (Number(item.channel_id) === Number(payload.channelId) && Number(item.credential_index) === Number(payload.credentialIndex))
      );
    }
    if (!account) {
      throw new Error('账号不存在或未加载');
    }
    const tabId = account.profile_key;
    if (this.tabs.has(tabId)) {
      await this.activateTab(tabId);
      return this.snapshot();
    }
    if (this.tabs.size >= MAX_BROWSER_TABS) {
      throw new Error(`最多同时打开 ${MAX_BROWSER_TABS} 个账号浏览器`);
    }

    const profile = this.ensureProfile(account);
    const partition = `persist:account-browser:${account.channel_id}:${account.credential_index}`;
    const proxy = parseProxyRules(account.proxy_rules || '');
    const ses = session.fromPartition(partition, { cache: true });
    await ses.setProxy(proxy.rules ? { proxyRules: proxy.rules } : { mode: 'direct' });
    await ses.forceReloadProxyConfig();
    ses.setPermissionRequestHandler((_webContents, permission, callback) => {
      callback(permission === 'geolocation' || permission === 'notifications');
    });
    browserWorkspaceSessionProfiles.set(partition, profile);
    if (!browserWorkspaceHeaderHookPartitions.has(partition)) {
      browserWorkspaceHeaderHookPartitions.add(partition);
      ses.webRequest.onBeforeSendHeaders((details, callback) => {
        const currentProfile = browserWorkspaceSessionProfiles.get(partition);
        if (currentProfile?.language) {
          details.requestHeaders['Accept-Language'] = currentProfile.language;
        }
        callback({ requestHeaders: details.requestHeaders });
      });
    }

    const view = new this.viewClass({
      webPreferences: {
        partition,
        nodeIntegration: false,
        contextIsolation: true,
        sandbox: true,
        webSecurity: true
      }
    });
    if (this.usingWebContentsView) {
      this.window.contentView.addChildView(view);
    } else {
      this.window.addBrowserView(view);
    }
    view.setBounds({ x: 0, y: 0, width: 0, height: 0 });
    if (proxy.auth) {
      browserWorkspaceProxyAuthByWebContentsId.set(view.webContents.id, proxy.auth);
      view.webContents.once('destroyed', () => {
        browserWorkspaceProxyAuthByWebContentsId.delete(view.webContents.id);
      });
    }
    view.webContents.setWindowOpenHandler(({ url }) => {
      view.webContents.loadURL(url);
      return { action: 'deny' };
    });
    view.webContents.on('did-start-loading', () => this.updateTab(tabId, { loading: true }));
    view.webContents.on('did-stop-loading', () => {
      this.updateTab(tabId, { loading: false, url: view.webContents.getURL(), title: view.webContents.getTitle() });
      this.captureTab(tabId).catch(() => {});
    });
    view.webContents.on('page-title-updated', (_event, title) => this.updateTab(tabId, { title }));
    view.webContents.on('did-navigate', (_event, url) => {
      this.updateTab(tabId, { url });
      this.rememberLastURL(tabId, url);
    });
    view.webContents.on('did-navigate-in-page', (_event, url) => {
      this.updateTab(tabId, { url });
      this.rememberLastURL(tabId, url);
    });

    const url = payload.url || this.profileStore.profiles?.[tabId]?.lastURL || account.open_url || 'https://chatgpt.com/';
    this.tabs.set(tabId, {
      id: tabId,
      account,
      accountSafe: sanitizeAccountForRenderer(account),
      profile,
      view,
      title: account.credential_label || account.credential_uid || tabId,
      url,
      loading: true,
      thumbnail: '',
      fingerprintStatus: {
        applying: true,
        applied: false,
        warnings: []
      }
    });
    await this.activateTab(tabId);
    try {
      const result = await applyFingerprintEmulation(view.webContents, profile);
      this.updateTab(tabId, {
        fingerprintStatus: {
          applying: false,
          applied: Boolean(result?.applied),
          warnings: Array.isArray(result?.warnings) ? result.warnings : []
        }
      });
    } catch (err) {
      this.updateTab(tabId, {
        fingerprintStatus: {
          applying: false,
          applied: false,
          warnings: [err.message || '指纹环境应用失败']
        }
      });
    }
    this.updateTab(tabId, { loading: true, url });
    view.webContents.loadURL(url).catch((err) => {
      this.updateTab(tabId, { loading: false, title: '加载失败', url });
      console.warn(`Failed to load account browser URL ${url}:`, err.message);
    });
    return this.snapshot();
  }

  ensureProfile(account) {
    const key = account.profile_key;
    if (!this.profileStore.profiles) {
      this.profileStore.profiles = {};
    }
    const profile = buildFingerprintProfile(account, this.profileStore.profiles[key]);
    this.profileStore.profiles[key] = {
      ...this.profileStore.profiles[key],
      ...profile
    };
    saveJSONFile(this.profilePath, this.profileStore);
    return profile;
  }

  rememberLastURL(tabId, url) {
    if (!tabId || !url || !this.profileStore.profiles?.[tabId]) {
      return;
    }
    this.profileStore.profiles[tabId].lastURL = url;
    saveJSONFile(this.profilePath, this.profileStore);
  }

  async activateTab(tabId) {
    if (!this.tabs.has(tabId)) {
      throw new Error('tab 不存在');
    }
    if (this.activeTabId && this.activeTabId !== tabId) {
      await this.captureTab(this.activeTabId).catch(() => {});
    }
    this.activeTabId = tabId;
    this.applyLayout();
    this.sendTabs();
    return this.snapshot();
  }

  async closeTab(tabId) {
    const tab = this.tabs.get(tabId);
    if (!tab) {
      return this.snapshot();
    }
    await this.captureTab(tabId).catch(() => {});
    browserWorkspaceProxyAuthByWebContentsId.delete(tab.view.webContents.id);
    if (this.usingWebContentsView) {
      this.window.contentView.removeChildView(tab.view);
    } else {
      this.window.removeBrowserView(tab.view);
    }
    tab.view.webContents.destroy();
    this.tabs.delete(tabId);
    if (this.activeTabId === tabId) {
      this.activeTabId = this.tabs.keys().next().value || '';
    }
    this.applyLayout();
    this.sendTabs();
    return this.snapshot();
  }

  async navigate(tabId, url) {
    const tab = this.tabs.get(tabId || this.activeTabId);
    if (!tab) {
      throw new Error('没有打开的浏览器');
    }
    const target = normalizeNavigationURL(url);
    await tab.view.webContents.loadURL(target);
    this.rememberLastURL(tab.id, target);
    return this.snapshot();
  }

  async reload(tabId) {
    const tab = this.tabs.get(tabId || this.activeTabId);
    if (tab) tab.view.webContents.reload();
    return this.snapshot();
  }

  async goBack(tabId) {
    const tab = this.tabs.get(tabId || this.activeTabId);
    if (tab && tab.view.webContents.canGoBack()) tab.view.webContents.goBack();
    return this.snapshot();
  }

  async goForward(tabId) {
    const tab = this.tabs.get(tabId || this.activeTabId);
    if (tab && tab.view.webContents.canGoForward()) tab.view.webContents.goForward();
    return this.snapshot();
  }

  setLayout(bounds) {
    this.bounds = normalizeBounds(bounds);
    this.applyLayout();
    return this.snapshot();
  }

  applyLayout() {
    for (const [tabId, tab] of this.tabs) {
      if (tabId === this.activeTabId) {
        if (typeof tab.view.setVisible === 'function') {
          tab.view.setVisible(true);
        }
        tab.view.setBounds(this.bounds);
      } else {
        if (typeof tab.view.setVisible === 'function') {
          tab.view.setVisible(false);
        }
        tab.view.setBounds({ x: 0, y: 0, width: 0, height: 0 });
      }
    }
  }

  async captureTab(tabId) {
    const tab = this.tabs.get(tabId);
    if (!tab || tab.view.webContents.isDestroyed()) {
      return '';
    }
    const image = await tab.view.webContents.capturePage();
    const size = image.getSize();
    if (!size.width || !size.height) {
      return '';
    }
    const resized = image.resize({ width: 220, height: 132, quality: 'best' });
    tab.thumbnail = resized.toDataURL();
    this.sendTabs();
    return tab.thumbnail;
  }

  updateTab(tabId, patch) {
    const tab = this.tabs.get(tabId);
    if (!tab) return;
    Object.assign(tab, patch);
    this.sendTabs();
  }

  snapshot() {
    return {
      activeTabId: this.activeTabId,
      maxTabs: MAX_BROWSER_TABS,
      tabs: Array.from(this.tabs.values()).map((tab) => ({
        id: tab.id,
        account: tab.accountSafe,
        profile: {
          fingerprintVersion: tab.profile.fingerprintVersion,
          userAgent: tab.profile.userAgent,
          language: tab.profile.language,
          timezone: tab.profile.timezone,
          viewport: tab.profile.viewport,
          screen: tab.profile.screen,
          browserPlatform: tab.profile.browserPlatform,
          hardwareConcurrency: tab.profile.hardwareConcurrency,
          deviceMemory: tab.profile.deviceMemory,
          webgl: tab.profile.webgl,
          webrtcPolicy: tab.profile.webrtcPolicy
        },
        title: tab.title,
        url: tab.url,
        loading: tab.loading,
        active: tab.id === this.activeTabId,
        thumbnail: tab.thumbnail,
        fingerprintStatus: tab.fingerprintStatus || {
          applying: false,
          applied: false,
          warnings: []
        }
      }))
    };
  }

  sendTabs() {
    if (!this.window || this.window.isDestroyed()) return;
    this.window.webContents.send('browser-workspace:tabs-updated', this.snapshot());
  }
}

function normalizeNavigationURL(value) {
  const raw = String(value || '').trim();
  if (!raw) return 'https://chatgpt.com/';
  if (/^[a-zA-Z][a-zA-Z\d+\-.]*:\/\//.test(raw)) {
    return raw;
  }
  return `https://${raw}`;
}

function normalizeBounds(bounds) {
  const out = {
    x: Math.max(0, Math.round(Number(bounds?.x) || 0)),
    y: Math.max(0, Math.round(Number(bounds?.y) || 0)),
    width: Math.max(0, Math.round(Number(bounds?.width) || 0)),
    height: Math.max(0, Math.round(Number(bounds?.height) || 0))
  };
  return out;
}

function registerBrowserWorkspaceIPC() {
  if (browserWorkspaceIPCRegistered) {
    return;
  }
  browserWorkspaceIPCRegistered = true;
  registerBrowserWorkspaceLoginHandler();
  ipcMain.handle('browser-workspace:list-accounts', async (_event, filters) => currentBrowserWorkspace().listAccounts(filters || {}));
  ipcMain.handle('browser-workspace:open-account', async (_event, payload) => currentBrowserWorkspace().openAccount(payload || {}));
  ipcMain.handle('browser-workspace:activate-tab', async (_event, tabId) => currentBrowserWorkspace().activateTab(tabId));
  ipcMain.handle('browser-workspace:close-tab', async (_event, tabId) => currentBrowserWorkspace().closeTab(tabId));
  ipcMain.handle('browser-workspace:navigate', async (_event, payload) => currentBrowserWorkspace().navigate(payload?.tabId, payload?.url));
  ipcMain.handle('browser-workspace:reload', async (_event, tabId) => currentBrowserWorkspace().reload(tabId));
  ipcMain.handle('browser-workspace:back', async (_event, tabId) => currentBrowserWorkspace().goBack(tabId));
  ipcMain.handle('browser-workspace:forward', async (_event, tabId) => currentBrowserWorkspace().goForward(tabId));
  ipcMain.handle('browser-workspace:set-layout', async (_event, bounds) => currentBrowserWorkspace().setLayout(bounds));
  ipcMain.handle('browser-workspace:get-tabs', async () => currentBrowserWorkspace().snapshot());
  ipcMain.handle('browser-workspace:open-admin', async () => {
    openAdminWindow();
    return true;
  });
}

function openAdminWindow() {
  const isDev = process.env.NODE_ENV === 'development';
  const loadPort = isDev ? DEV_FRONTEND_PORT : PORT;
  if (adminWindow && !adminWindow.isDestroyed()) {
    adminWindow.show();
    adminWindow.focus();
    return;
  }
  adminWindow = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 980,
    minHeight: 640,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      nodeIntegration: false,
      contextIsolation: true
    },
    title: 'New API Admin',
    icon: path.join(__dirname, 'icon.png')
  });
  adminWindow.loadURL(`http://127.0.0.1:${loadPort}`);
  adminWindow.on('closed', () => {
    adminWindow = null;
  });
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1440,
    height: 920,
    minWidth: 1180,
    minHeight: 760,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      nodeIntegration: false,
      contextIsolation: true
    },
    title: 'New API Browser Workspace',
    icon: path.join(__dirname, 'icon.png')
  });

  browserWorkspace = new BrowserWorkspace(mainWindow);
  registerBrowserWorkspaceIPC();
  mainWindow.loadFile(path.join(__dirname, 'workspace.html'));
  
  console.log('Loading Electron browser workspace');

  if (process.env.NODE_ENV === 'development') {
    mainWindow.webContents.openDevTools();
  }

  // Close to tray instead of quitting
  mainWindow.on('close', (event) => {
    if (!app.isQuitting) {
      event.preventDefault();
      mainWindow.hide();
      if (process.platform === 'darwin') {
        app.dock.hide();
      }
    }
  });

  mainWindow.on('closed', () => {
    browserWorkspace = null;
    mainWindow = null;
  });
}

function createTray() {
  // Use template icon for macOS (black with transparency, auto-adapts to theme)
  // Use colored icon for Windows
  const trayIconPath = process.platform === 'darwin'
    ? path.join(__dirname, 'tray-iconTemplate.png')
    : path.join(__dirname, 'tray-icon-windows.png');

  tray = new Tray(trayIconPath);

  const contextMenu = Menu.buildFromTemplate([
    {
      label: 'Show New API',
      click: () => {
        if (mainWindow === null) {
          createWindow();
        } else {
          mainWindow.show();
          if (process.platform === 'darwin') {
            app.dock.show();
          }
        }
      }
    },
    { type: 'separator' },
    {
      label: 'Quit',
      click: () => {
        app.isQuitting = true;
        app.quit();
      }
    }
  ]);

  tray.setToolTip('New API');
  tray.setContextMenu(contextMenu);

  // On macOS, clicking the tray icon shows the window
  tray.on('click', () => {
    if (mainWindow === null) {
      createWindow();
    } else {
      mainWindow.isVisible() ? mainWindow.hide() : mainWindow.show();
      if (mainWindow.isVisible() && process.platform === 'darwin') {
        app.dock.show();
      }
    }
  });
}

app.whenReady().then(async () => {
  try {
    await startServer();
    createTray();
    createWindow();
  } catch (err) {
    console.error('Failed to start application:', err);
    
    // 分析启动失败的错误
    const knownError = analyzeError(serverErrorLogs);
    
    if (knownError) {
      dialog.showMessageBox({
        type: 'error',
        title: knownError.title,
        message: `启动失败: ${knownError.message}`,
        detail: `${knownError.solution}\n\n━━━━━━━━━━━━━━━━━━━━━━\n\n错误信息: ${err.message}\n\n错误类型: ${knownError.type}`,
        buttons: ['退出', '查看完整日志'],
        defaultId: 0,
        cancelId: 0
      }).then((result) => {
        if (result.response === 1) {
          // 用户选择查看日志
          const logPath = saveAndOpenErrorLog();
          
          const confirmMessage = logPath 
            ? `日志已保存到:\n${logPath}\n\n日志文件已在默认文本编辑器中打开。\n\n点击"退出"关闭应用程序。`
            : '日志保存失败，但已在控制台输出。\n\n点击"退出"关闭应用程序。';
          
          dialog.showMessageBox({
            type: 'info',
            title: '日志已保存',
            message: confirmMessage,
            buttons: ['退出'],
            defaultId: 0
          }).then(() => {
            app.quit();
          });
          
          console.log('=== 完整错误日志 ===');
          console.log(serverErrorLogs.join('\n'));
        } else {
          app.quit();
        }
      });
    } else {
      dialog.showMessageBox({
        type: 'error',
        title: '启动失败',
        message: '无法启动服务器',
        detail: `错误信息: ${err.message}\n\n请检查日志获取更多信息。`,
        buttons: ['退出', '查看完整日志'],
        defaultId: 0,
        cancelId: 0
      }).then((result) => {
        if (result.response === 1) {
          // 用户选择查看日志
          const logPath = saveAndOpenErrorLog();
          
          const confirmMessage = logPath 
            ? `日志已保存到:\n${logPath}\n\n日志文件已在默认文本编辑器中打开。\n\n点击"退出"关闭应用程序。`
            : '日志保存失败，但已在控制台输出。\n\n点击"退出"关闭应用程序。';
          
          dialog.showMessageBox({
            type: 'info',
            title: '日志已保存',
            message: confirmMessage,
            buttons: ['退出'],
            defaultId: 0
          }).then(() => {
            app.quit();
          });
          
          console.log('=== 完整错误日志 ===');
          console.log(serverErrorLogs.join('\n'));
        } else {
          app.quit();
        }
      });
    }
  }
});

app.on('window-all-closed', () => {
  // Don't quit when window is closed, keep running in tray
  // Only quit when explicitly choosing Quit from tray menu
});

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow();
  }
});

app.on('before-quit', (event) => {
  if (serverProcess) {
    event.preventDefault();

    console.log('Shutting down server...');
    serverProcess.kill('SIGTERM');

    setTimeout(() => {
      if (serverProcess) {
        serverProcess.kill('SIGKILL');
      }
      app.exit();
    }, 5000);

    serverProcess.on('close', () => {
      serverProcess = null;
      app.exit();
    });
  }
});
