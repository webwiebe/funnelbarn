import type { FeatureFlag } from '../../lib/api'
import { C } from './flagFormTheme'
import { useFlagForm } from './useFlagForm'
import {
  ConfigFields, ConversionField, ErrorBanner, ExperimentFields, FlagKindSelector, IdentityFields,
  ModalFooter, ModalHeader, TargetingSection,
} from './FlagFormParts'

interface FlagFormModalProps {
  projectId: string
  flag?: FeatureFlag
  onClose: () => void
  onSaved: (f: FeatureFlag) => void
}

export function FlagFormModal({ projectId, flag, onClose, onSaved }: FlagFormModalProps) {
  const form = useFlagForm(projectId, flag, onSaved)
  const { flagKind, setFlagKind } = form.config

  return (
    <>
      <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', zIndex: 1000 }} />
      <div style={{
        position: 'fixed', top: '50%', left: '50%', transform: 'translate(-50%, -50%)',
        background: C.surface, border: `1px solid ${C.border}`, borderRadius: 16,
        width: '90%', maxWidth: 520, zIndex: 1001, maxHeight: '90vh', overflowY: 'auto',
      }}>
        <ModalHeader isEdit={form.isEdit} onClose={onClose} />

        <div style={{ padding: '1.5rem' }}>
          <ErrorBanner message={form.error} />
          <IdentityFields form={form} />
          <FlagKindSelector value={flagKind} onChange={setFlagKind} />
          {flagKind === 'config' ? <ConfigFields form={form} flag={flag} /> : <ExperimentFields form={form} />}
          {flagKind === 'experiment' && <ConversionField form={form} />}
          <TargetingSection form={form} />
        </div>

        <ModalFooter form={form} onClose={onClose} />
      </div>
    </>
  )
}
