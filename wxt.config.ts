import { defineConfig } from 'wxt';

// See https://wxt.dev/api/config.html
export default defineConfig({
  modules: ['@wxt-dev/module-react'],
  outDir: 'build',
  imports: {
    eslintrc: {
      enabled: 9,
    },
  },

  manifest: {
    name: '__MSG_extension_name__',
    description: '__MSG_description__',
    default_locale: 'zh_CN',
    version: '3.0.2',
    author: 'fydeos',

    icons: {
      16: '/icon-16.png',
      32: '/icon-32.png',
      48: '/icon-48.png',
      64: '/icon-64.png',
      128: '/icon-128.png'
    },

    action: {
      default_icon: {
        16: '/icon-16.png',
        32: '/icon-32.png',
        48: '/icon-48.png',
        64: '/icon-64.png',
        128: '/icon-128.png'
      }
    },

    permissions: [
      'storage',
      'alarms',
      'unlimitedStorage',
      'input',
      'virtualKeyboardPrivate',
      'inputMethodPrivate'
    ],
    host_permissions: [
      'https://api.deepseek.com/*',
      'https://shulufa.555044.xyz/*'
    ],

    content_security_policy: {
      extension_pages: "script-src 'self' 'wasm-unsafe-eval'; object-src 'self'; connect-src 'self' https://api.deepseek.com https://shulufa.555044.xyz;"
    },

    input_components: [
      {
        name: '__MSG_input_method_name__',
        id: 'fyde-rhythm',
        indicator: '真',
        language: ['zh-CN', 'zh', 'en', 'jp', 'ko', 'zh-TW'],
        layouts: ['us'],
        input_view: 'inputview/inputview.html'
      }
    ],

    update_url: 'https://store.fydeos.com/update/nfglebjgiflmmcdddkbcbgmdkomlfcpa/updates.xml'
  } as any,

  vite: () => ({
    build: {
      assetsInlineLimit: 0,
      chunkSizeWarningLimit: 1500,
      rollupOptions: {
        output: {
          assetFileNames: (assetInfo) => {
            if (assetInfo.name?.endsWith('.wasm')) {
              return '[name].[ext]';
            }
            return 'assets/[name]-[hash].[ext]';
          }
        },
        onwarn(warning, warn) {
          if (
            warning.code === 'EVAL' &&
            warning.id?.includes('lottie-web')
          ) {
            return;
          }
          warn(warning);
        }
      }
    }
  })
});
