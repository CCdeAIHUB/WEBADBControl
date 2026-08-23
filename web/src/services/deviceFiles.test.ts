import { describe, expect, it } from 'vitest'
import { formatFileSize, joinRemotePath, parentRemotePath, pathSegments, sortFileEntries } from './deviceFiles'

describe('deviceFiles helpers', () => {
  it('keeps remote paths normalized for device file operations', () => {
    // 场景：UI 组合目录和文件名时，必须保持设备绝对路径，不能生成双斜杠或空路径。
    expect(joinRemotePath('/sdcard/', 'Download')).toBe('/sdcard/Download')
    expect(parentRemotePath('/sdcard/Download/report.txt')).toBe('/sdcard/Download')
    expect(parentRemotePath('/sdcard')).toBe('/sdcard')
  })

  it('builds breadcrumbs and sorts directories before files', () => {
    // 场景：文件管理页要接近资源管理器体验，目录优先且路径层级可点击。
    expect(pathSegments('/sdcard/DCIM/Camera')).toEqual([
      { label: '/sdcard', path: '/sdcard' },
      { label: 'DCIM', path: '/sdcard/DCIM' },
      { label: 'Camera', path: '/sdcard/DCIM/Camera' },
    ])
    expect(sortFileEntries([
      { name: 'b.txt', path: '/sdcard/b.txt', type: 'file', permissions: '-', size: 1 },
      { name: 'A', path: '/sdcard/A', type: 'directory', permissions: 'd', size: 0 },
    ]).map(item => item.name)).toEqual(['A', 'b.txt'])
  })

  it('formats file sizes for compact table display', () => {
    expect(formatFileSize(0)).toBe('—')
    expect(formatFileSize(2048)).toBe('2.0 KB')
    expect(formatFileSize(12 * 1024 * 1024)).toBe('12 MB')
  })
})
