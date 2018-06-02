import * as React from 'react'
import { RouteComponentProps } from 'react-router'
import { Subject, Subscription } from 'rxjs'
import { catchError, mergeMap, tap } from 'rxjs/operators'
import { EmailInput, UsernameInput } from '../auth/SignInSignUpCommon'
import { CopyableText } from '../components/CopyableText'
import { Form } from '../components/Form'
import { PageTitle } from '../components/PageTitle'
import { eventLogger } from '../tracking/eventLogger'
import { createUser } from './backend'

interface Props extends RouteComponentProps<any> {}

export interface State {
    errorDescription?: string
    loading: boolean

    /**
     * The password reset URL generated for the new user account.
     */
    newUserPasswordResetURL?: string | null

    // Form
    username: string
    email: string
}

/**
 * A page with a form to invite a user to the site.
 */
export class SiteAdminInviteUserPage extends React.Component<Props, State> {
    public state: State = {
        loading: false,
        username: '',
        email: '',
    }

    private submits = new Subject<{ username: string; email: string }>()
    private subscriptions = new Subscription()

    public componentDidMount(): void {
        eventLogger.logViewEvent('SiteAdminInviteUser')

        this.subscriptions.add(
            this.submits
                .pipe(
                    tap(() =>
                        this.setState({
                            loading: true,
                            errorDescription: undefined,
                        })
                    ),
                    mergeMap(({ username, email }) =>
                        createUser(username, email).pipe(
                            catchError(error => {
                                console.error(error)
                                this.setState({ loading: false, errorDescription: error.message })
                                return []
                            })
                        )
                    )
                )
                .subscribe(
                    ({ resetPasswordURL }) =>
                        this.setState({
                            loading: false,
                            errorDescription: undefined,
                            newUserPasswordResetURL: resetPasswordURL,
                        }),
                    error => console.error(error)
                )
        )
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        return (
            <div className="site-admin-invite-user-page">
                <PageTitle title="Invite user - Admin" />
                <h2>Invite user</h2>
                <p>
                    Create a new user account and generate a password reset link. You must manually send the link to the
                    new user.
                </p>
                {this.state.newUserPasswordResetURL ? (
                    <div className="alert alert-success">
                        <p>
                            Account created for <strong>{this.state.username}</strong>.
                        </p>
                        {this.state.newUserPasswordResetURL !== null && (
                            <>
                                <p>You must manually send this password reset link to the new user:</p>
                                <CopyableText text={this.state.newUserPasswordResetURL} size={40} />
                            </>
                        )}
                        <button className="btn btn-primary mt-2" onClick={this.dismissAlert}>
                            Invite another user
                        </button>
                    </div>
                ) : (
                    <Form onSubmit={this.onSubmit} className="site-admin-invite-user-page__form">
                        <div className="form-group">
                            <label>Username</label>
                            <UsernameInput
                                onChange={this.onUsernameFieldChange}
                                value={this.state.username}
                                required={true}
                                disabled={this.state.loading}
                                autoFocus={true}
                            />
                        </div>
                        <div className="form-group">
                            <label>Email</label>
                            <EmailInput
                                onChange={this.onEmailFieldChange}
                                value={this.state.email}
                                disabled={this.state.loading}
                            />
                        </div>
                        {this.state.errorDescription && (
                            <div className="alert alert-danger my-2">{this.state.errorDescription}</div>
                        )}
                        <button className="btn btn-primary" disabled={this.state.loading} type="submit">
                            Generate password reset link
                        </button>
                    </Form>
                )}
                <hr />
                <p>
                    See <a href="https://about.sourcegraph.com/docs/server/config/">Sourcegraph documentation</a> for
                    information about configuring user accounts and SSO authentication.
                </p>
            </div>
        )
    }

    private onEmailFieldChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        this.setState({ email: e.target.value, errorDescription: undefined })
    }

    private onUsernameFieldChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        this.setState({ username: e.target.value, errorDescription: undefined })
    }

    private onSubmit = (event: React.FormEvent<HTMLFormElement>): void => {
        event.preventDefault()
        event.stopPropagation()
        this.submits.next({ username: this.state.username, email: this.state.email })
    }

    private dismissAlert = () =>
        this.setState({
            newUserPasswordResetURL: undefined,
            errorDescription: undefined,
            username: '',
            email: '',
        })
}
