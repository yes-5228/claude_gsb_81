// 保存当前筛选条件为常用筛选的弹窗。
import { useState } from 'react';
import { Modal } from '../../components/Modal';

interface SaveFilterDialogProps {
  open: boolean;
  saving: boolean;
  onClose: () => void;
  onConfirm: (name: string) => void;
}

export function SaveFilterDialog({ open, saving, onClose, onConfirm }: SaveFilterDialogProps) {
  const [name, setName] = useState('');

  const submit = () => {
    const trimmed = name.trim();
    if (!trimmed) {
      return;
    }
    onConfirm(trimmed);
    setName('');
  };

  return (
    <Modal
      open={open}
      title="保存常用筛选"
      onClose={() => {
        setName('');
        onClose();
      }}
      footer={
        <>
          <button
            type="button"
            className="btn btn-ghost"
            onClick={() => {
              setName('');
              onClose();
            }}
          >
            取消
          </button>
          <button type="button" className="btn btn-primary" disabled={!name.trim() || saving} onClick={submit}>
            {saving ? '保存中…' : '保存'}
          </button>
        </>
      }
    >
      <div className="form-field">
        <label className="form-label">条件名称</label>
        <input
          className="input"
          placeholder="例如：城东片区 · 汛期专项 · 清淤中"
          maxLength={30}
          value={name}
          autoFocus
          onChange={(event) => setName(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              submit();
            }
          }}
        />
        <p className="form-hint">只保存当前筛选条件，不包含页码；最多保留 20 条。</p>
      </div>
    </Modal>
  );
}
