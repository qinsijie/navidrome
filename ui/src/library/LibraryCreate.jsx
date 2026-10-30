import React, { useCallback, useEffect, useState } from 'react'
import PropTypes from 'prop-types'
import {
  Create,
  SimpleForm,
  TextInput,
  BooleanInput,
  required,
  useTranslate,
  useMutation,
  useNotify,
  useRedirect,
  useInput,
} from 'react-admin'
import { TextField, Typography } from '@material-ui/core'
import Autocomplete from '@material-ui/lab/Autocomplete'
import { makeStyles } from '@material-ui/core/styles'
import { Title } from '../common'
import { PIDInputs } from './PIDInput'
import { httpClient } from '../dataProvider'

const PathInput = ({ source, validate, fullWidth }) => {
  const {
    input: { value, onChange },
    meta: { touched, error },
  } = useInput({ source, validate })
  const translate = useTranslate()
  const [options, setOptions] = useState([])

  useEffect(() => {
    httpClient('/api/system/paths')
      .then(({ json }) => {
        if (Array.isArray(json)) {
          setOptions(json)
        }
      })
      .catch(() => {})
  }, [])

  return (
    <Autocomplete
      freeSolo
      options={options}
      getOptionLabel={(option) =>
        typeof option === 'string'
          ? option
          : option.name
            ? `${option.name} (${option.path})`
            : option.path || ''
      }
      value={value || ''}
      onInputChange={(event, newInputValue) => {
        onChange(newInputValue)
      }}
      onChange={(event, newValue) => {
        if (typeof newValue === 'string') {
          onChange(newValue)
        } else if (newValue && newValue.path) {
          onChange(newValue.path)
        } else {
          onChange('')
        }
      }}
      renderInput={(params) => (
        <TextField
          {...params}
          label={translate('resources.library.fields.path')}
          variant="outlined"
          margin="dense"
          fullWidth={fullWidth}
          error={!!(touched && error)}
          helperText={
            touched && error
              ? translate(error)
              : '选择飞牛授权存储卷路径或直接输入物理路径'
          }
        />
      )}
    />
  )
}

PathInput.propTypes = {
  source: PropTypes.string.isRequired,
  validate: PropTypes.array,
  fullWidth: PropTypes.bool,
}

const useStyles = makeStyles((theme) => ({
  spaced: { marginTop: theme.spacing(3) },
}))

// SimpleForm passes form props (variant, record, ...) to its children, so Typography can't be used directly
const SectionTitle = ({ label, spaced }) => {
  const translate = useTranslate()
  const classes = useStyles()
  return (
    <Typography
      variant="h6"
      gutterBottom
      className={spaced ? classes.spaced : undefined}
    >
      {translate(label)}
    </Typography>
  )
}

SectionTitle.propTypes = {
  label: PropTypes.string.isRequired,
  spaced: PropTypes.bool,
}

const LibraryCreate = (props) => {
  const translate = useTranslate()
  const [mutate] = useMutation()
  const notify = useNotify()
  const redirect = useRedirect()
  const resourceName = translate('resources.library.name', { smart_count: 1 })
  const title = translate('ra.page.create', {
    name: `${resourceName}`,
  })

  const save = useCallback(
    async (values) => {
      try {
        await mutate(
          {
            type: 'create',
            resource: 'library',
            payload: { data: values },
          },
          { returnPromise: true },
        )
        notify('resources.library.notifications.created', 'info', {
          smart_count: 1,
        })
        redirect('/library')
      } catch (error) {
        // Handle validation errors with proper field mapping
        if (error.body && error.body.errors) {
          return error.body.errors
        }

        // Handle other structured errors from the server
        if (error.body && error.body.error) {
          const errorMsg = error.body.error

          // Handle database constraint violations
          if (errorMsg.includes('UNIQUE constraint failed: library.name')) {
            return { name: 'ra.validation.unique' }
          }
          if (errorMsg.includes('UNIQUE constraint failed: library.path')) {
            return { path: 'ra.validation.unique' }
          }

          // Show a general notification for other server errors
          notify(errorMsg, 'error')
          return
        }

        // Fallback for unexpected error formats
        const fallbackMessage =
          error.message ||
          (typeof error === 'string' ? error : 'An unexpected error occurred')
        notify(fallbackMessage, 'error')
      }
    },
    [mutate, notify, redirect],
  )

  return (
    <Create title={<Title subTitle={title} />} {...props}>
      <SimpleForm save={save} variant={'outlined'}>
        <SectionTitle label="resources.library.sections.basic" />
        <TextInput source="name" validate={[required()]} />
        <PathInput source="path" validate={[required()]} fullWidth />
        <BooleanInput source="defaultNewUsers" />
        <SectionTitle label="resources.library.sections.pid" spaced />
        <PIDInputs />
      </SimpleForm>
    </Create>
  )
}

export default LibraryCreate
