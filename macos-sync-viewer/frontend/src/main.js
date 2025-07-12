// 导入Go后端暴露的方法
// 我们只使用 GetLatestClipboard 来获取最新数据
const { GetLatestClipboard, GetLatestScreenshot } = window.go.main.App;
import { Quit, WindowMinimise, WindowToggleMaximise } from '../wailsjs/runtime/runtime.js';

// --- 全局变量，用于在前端存储历史记录 ---
let clipboardHistory = []; // 存储历史记录的数组
const MAX_HISTORY_SIZE = 50; // 定义最大历史记录条数

// --- 获取DOM元素 ---
const screenshotImg = document.getElementById('screenshot-img');
const screenshotPlaceholder = document.getElementById('screenshot-placeholder');
const screenshotStatus = document.getElementById('screenshot-status');
const clipboardHistoryList = document.getElementById('clipboard-history-list');
const clipboardStatus = document.getElementById('clipboard-status');

// --- 窗口控制事件绑定 ---
document.addEventListener('DOMContentLoaded', () => {
    const closeBtn = document.getElementById('closeBtn');
    const minBtn = document.getElementById('minBtn');
    const maxBtn = document.getElementById('maxBtn');

    if (closeBtn && minBtn && maxBtn) {
        closeBtn.addEventListener('click', () => Quit());
        minBtn.addEventListener('click', () => WindowMinimise());
        maxBtn.addEventListener('click', () => WindowToggleMaximise());
    } else {
        console.error("Control buttons not found!");
    }

    const cardContent = document.querySelector('#screenshot-container .card-content');

    if (cardContent) {
        cardContent.style.maxHeight = 'none';
    }
});

/**
 * 渲染剪贴板历史列表
 * 此函数读取全局的 clipboardHistory 数组并更新UI
 */
function renderClipboardHistory() {
    // 清空当前列表
    clipboardHistoryList.innerHTML = '';

    if (clipboardHistory.length > 0) {
        // 遍历历史记录并创建DOM元素
        clipboardHistory.forEach(item => {
            const historyItem = document.createElement('div');
            historyItem.className = 'history-item';

            const contentEl = document.createElement('pre');
            contentEl.textContent = item.content;

            const timestampEl = document.createElement('span');
            timestampEl.className = 'timestamp';
            const date = new Date(item.timestamp);
            timestampEl.textContent = `${date.toLocaleDateString()} ${date.toLocaleTimeString()}`;

            historyItem.appendChild(contentEl);
            historyItem.appendChild(timestampEl);

            clipboardHistoryList.appendChild(historyItem);
        });
        // 更新状态为最近一条记录的时间
        const latestDate = new Date(clipboardHistory[0].timestamp);
        clipboardStatus.textContent = `更新于: ${latestDate.toLocaleTimeString()}`;
    } else {
        // 如果没有历史记录
        clipboardHistoryList.innerHTML = '<div class="placeholder">剪贴板历史为空</div>';
        clipboardStatus.textContent = '暂无记录';
    }
}


/**
 * 轮询并更新剪贴板历史
 * 这是核心函数，用于获取新数据、与现有历史比较并决定是否更新
 */
function pollAndupdateClipboardHistory() {
    GetLatestClipboard()
        .then(response => {
            const newContent = response.content;

            // 检查返回的内容是否有效，以及是否与最新一条历史记录重复
            if (newContent && (clipboardHistory.length === 0 || clipboardHistory[0].content !== newContent)) {

                // 创建新的历史记录项
                const newEntry = {
                    content: newContent,
                    timestamp: new Date(response.timestamp) // 使用后端提供的时间戳
                };

                // 将新项添加到历史记录数组的开头
                clipboardHistory.unshift(newEntry);

                // 如果历史记录超过最大限制，则移除最旧的一条
                if (clipboardHistory.length > MAX_HISTORY_SIZE) {
                    clipboardHistory.pop();
                }

                // 因为历史记录已更新，所以重新渲染UI
                renderClipboardHistory();
            }
        })
        .catch(err => {
            console.error('获取剪贴板失败:', err);
            clipboardStatus.textContent = '更新失败';
        });
}


// 更新屏幕截图的函数
function updateScreenshot() {
    screenshotStatus.textContent = '正在更新...';
    GetLatestScreenshot()
        .then(base64Image => {
            // 成功获取图片
            if (base64Image) {
                screenshotImg.src = base64Image;
                screenshotImg.style.display = 'block'; // 显示图片
                screenshotPlaceholder.style.display = 'none'; // 隐藏占位符
                screenshotStatus.textContent = `更新于: ${new Date().toLocaleTimeString()}`;
            } else {
                // 如果返回为空，则显示占位符
                screenshotImg.style.display = 'none';
                screenshotPlaceholder.style.display = 'block';
                screenshotStatus.textContent = '未找到截图';
            }
        })
        .catch(err => {
            // 获取失败
            console.error('获取截图失败:', err);
            screenshotStatus.textContent = '更新失败';
            screenshotImg.style.display = 'none';
            screenshotPlaceholder.style.display = 'block';
            screenshotPlaceholder.textContent = `错误: ${err}`;
        });
}

// --- 初始化和定时器 ---
document.addEventListener('DOMContentLoaded', () => {
    // 立即执行一次以填充初始数据
    pollAndupdateClipboardHistory();
    updateScreenshot();

    // 设置定时器，每2秒轮询一次剪贴板
    setInterval(pollAndupdateClipboardHistory, 2000);
    // 设置定时器，每10秒请求一次新截图
    setInterval(updateScreenshot, 10000);
});